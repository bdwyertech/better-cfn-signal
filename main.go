// Encoding: UTF-8
//
// Better CFN Signal
//
// Copyright © 2020 Brian Dwyer - Intelligent Digital Services
//

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/aws/aws-sdk-go/service/ec2"
)

var healthcheckUrl string
var healthcheckTimeout time.Duration
var signalFailure bool

func init() {
	flag.StringVar(&healthcheckUrl, "healthcheck-url", "", "Healthcheck endpoint URL")
	flag.DurationVar(&healthcheckTimeout, "healthcheck-timeout", 5*time.Minute, "Healthcheck timeout")
	flag.BoolVar(&signalFailure, "failure", false, "Signal resource failure")

	if _, ok := os.LookupEnv("CFN_SIGNAL_DEBUG"); ok {
		log.SetLevel(log.DebugLevel)
		log.SetReportCaller(true)
	}
}

func main() {
	flag.Parse()

	if versionFlag {
		showVersion()
		os.Exit(0)
	}

	// AWS Session
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		Config:            *aws.NewConfig().WithCredentialsChainVerboseErrors(true),
		SharedConfigState: session.SharedConfigDisable,
	}))

	metadata := ec2metadata.New(sess)

	if !metadata.Available() {
		log.Fatal("EC2 Metadata is not available... Are we running on an EC2 instance?")
	}

	identity, err := metadata.GetInstanceIdentityDocument()
	if err != nil {
		log.Fatal(err)
	}
	instanceID := identity.InstanceID
	sess.Config = sess.Config.WithRegion(identity.Region)

	ec2client := ec2.New(sess)

	input := &ec2.DescribeTagsInput{
		Filters: []*ec2.Filter{
			&ec2.Filter{
				Name: aws.String("resource-id"),
				Values: []*string{
					aws.String(instanceID),
				},
			},
		},
	}

	resp, err := ec2client.DescribeTags(input)
	if err != nil {
		log.Fatal(err)
	}

	tags := resp.Tags

	// Handle EC2 API Pagination
	for {
		if resp.NextToken == nil {
			break
		}

		input.NextToken = resp.NextToken

		resp, err := ec2client.DescribeTags(input)
		if err != nil {
			log.Fatal(err)
		}

		tags = append(tags, resp.Tags...)
	}

	var LogicalID, StackName *string

	for _, tag := range tags {
		switch *tag.Key {
		case "aws:cloudformation:logical-id":
			LogicalID = tag.Value
		case "aws:cloudformation:stack-name":
			StackName = tag.Value
		}
	}

	if LogicalID == nil || StackName == nil {
		log.Fatal("Required tags were not present on EC2 Instance!")
	}

	cfclient := cloudformation.New(sess)

	signal := &cloudformation.SignalResourceInput{
		LogicalResourceId: LogicalID,
		StackName:         StackName,
		Status: func() *string {
			if signalFailure {
				return aws.String("FAILURE")
			}
			return aws.String("SUCCESS")
		}(),
		UniqueId: aws.String(instanceID),
	}

	// Wait for Healthcheck if configured
	if !signalFailure && healthcheckUrl != "" {
		waitUntilHealthy()
	}

	cfr, err := cfclient.SignalResource(signal)
	// Error Handling
	// We don't want to have a non-zero exit code cause cloud-init unit failure during autoscaling operations
	if err != nil {
		func() {
			if awsErr, ok := err.(awserr.Error); ok {
				if awsErr.Code() == "ValidationError" {
					if strings.HasSuffix(awsErr.Message(), "is in CREATE_COMPLETE state and cannot be signaled") {
						log.Warn(awsErr)
						return
					}
				}
			}
			log.Fatal(err)
		}()
	}

	log.Println("SignalResource Response:", cfr)
}

func waitUntilHealthy() {

	// Copy of http.DefaultTransport with Flippable TLS Verification
	// https://golang.org/pkg/net/http/#Client
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: func() bool {
				_, ok := os.LookupEnv("CFN_SIGNAL_SSL_VERIFY")
				return ok
			}()},
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
				DualStack: true,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), healthcheckTimeout)
	defer cancel()

	for {
		req, err := http.NewRequestWithContext(ctx, "GET", healthcheckUrl, nil)
		if err != nil {
			log.Fatal(err)
		}
		requestTimeout := 30 * time.Second
		rctx, rcancel := context.WithTimeout(ctx, requestTimeout)
		defer rcancel()
		resp, err := client.Do(req.WithContext(rctx))
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr == context.DeadlineExceeded {
				log.Fatal(fmt.Errorf("healthcheck exceeded timeout(%s): %w", healthcheckTimeout, err))
			}
			if ctxErr := rctx.Err(); ctxErr == context.DeadlineExceeded {
				log.Warn(fmt.Errorf("healthcheck request timeout(%s): %w", requestTimeout, err))
			} else {
				log.Error(err)
			}
			time.Sleep(5 * time.Second)
			continue
		}
		defer resp.Body.Close()
		switch resp.StatusCode {
		case 200:
			return
		default:
			log.Warnf("%v :: (%v) %v", healthcheckUrl, resp.StatusCode, resp.Status)
			time.Sleep(5 * time.Second)
			continue
		}
	}
}
