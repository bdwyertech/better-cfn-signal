// Encoding: UTF-8
//
// Better CFN Signal
//
// Copyright © 2023 Brian Dwyer - Intelligent Digital Services
//

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cloudformationtypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	"github.com/mattn/go-isatty"
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

	// Workaround for https://github.com/PowerShell/PowerShell/issues/14273
	// PSNotApplyErrorActionToStderr
	if runtime.GOOS == "windows" && !(isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())) {
		log.SetOutput(os.Stdout)
	}
}

func main() {
	flag.Parse()

	if versionFlag {
		showVersion()
		os.Exit(0)
	}

	imdsClient := imds.New(imds.Options{})

	ctx := context.Background()

	awsCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	idDoc, err := imdsClient.GetInstanceIdentityDocument(awsCtx, &imds.GetInstanceIdentityDocumentInput{})
	if err != nil {
		log.Fatalf("failed to get instance identity document: %v", err)
	}
	instanceID := idDoc.InstanceID

	cfg := aws.Config{
		Region: idDoc.Region,
		// We should only ever be using this on EC2 Instances with an Instance Role...
		Credentials: ec2rolecreds.New(),
	}

	var tags []ec2types.TagDescription
	ec2Client := ec2.NewFromConfig(cfg)
	paginator := ec2.NewDescribeTagsPaginator(ec2Client, &ec2.DescribeTagsInput{
		Filters: []ec2types.Filter{
			{
				Name:   aws.String("resource-id"),
				Values: []string{instanceID},
			},
		},
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			log.Fatalf("failed to describe EC2 tags: %v", err)
		}
		tags = append(tags, page.Tags...)
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

	cfclient := cloudformation.NewFromConfig(cfg)

	signal := &cloudformation.SignalResourceInput{
		LogicalResourceId: LogicalID,
		StackName:         StackName,
		Status: func() cloudformationtypes.ResourceSignalStatus {
			if signalFailure {
				return cloudformationtypes.ResourceSignalStatusFailure
			}
			return cloudformationtypes.ResourceSignalStatusSuccess
		}(),
		UniqueId: aws.String(instanceID),
	}

	// Wait for Healthcheck if configured
	if !signalFailure && healthcheckUrl != "" {
		waitUntilHealthy()
	}

	cfr, err := cfclient.SignalResource(ctx, signal)
	// Error Handling
	// We don't want to have a non-zero exit code cause cloud-init unit failure during autoscaling operations
	if err != nil {
		func() {
			var invalidStatus *cloudformationtypes.InvalidChangeSetStatusException
			if errors.As(err, &invalidStatus) {
				log.Warn(invalidStatus.Error())
				return
			}

			var apiErr smithy.APIError
			if errors.As(err, &apiErr) {
				if apiErr.ErrorCode() == "ValidationError" && strings.HasSuffix(apiErr.ErrorMessage(), "state and cannot be signaled") {
					log.Warn(apiErr.ErrorMessage())
					return
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

	var bodyBytes []byte

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
				if len(bodyBytes) > 0 {
					var prettyJSON bytes.Buffer
					if err := json.Indent(&prettyJSON, bodyBytes, "", "  "); err != nil {
						log.Error(string(bodyBytes))
					} else {
						log.Error(prettyJSON.String())
					}
				}
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
			bodyBytes, _ = io.ReadAll(resp.Body)
			resp.Body.Close()
			time.Sleep(5 * time.Second)
			continue
		}
	}
}
