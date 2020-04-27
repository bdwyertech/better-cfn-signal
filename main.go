// Encoding: UTF-8

package main

import (
	"flag"
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/aws/aws-sdk-go/service/ec2"
)

var verFlag = flag.Bool("version", false, "Display version")
var failFlag = flag.Bool("failure", false, "Signal resource failure")

var GitCommit string
var ReleaseVer string
var ReleaseDate string

func init() {
	if _, ok := os.LookupEnv("CFN_SIGNAL_DEBUG"); ok {
		log.SetLevel(log.DebugLevel)
		log.SetReportCaller(true)
	}
}

func main() {
	flag.Parse()

	if *verFlag {
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
			if *failFlag {
				return aws.String("FAILURE")
			}
			return aws.String("SUCCESS")
		}(),
		UniqueId: aws.String(instanceID),
	}

	cfr, err := cfclient.SignalResource(signal)
	if err != nil {
		log.Fatal(err)
	}
	log.Println(cfr)
}

func showVersion() {
	if GitCommit == "" {
		GitCommit = "DEVELOPMENT"
	}
	if ReleaseVer == "" {
		ReleaseVer = "DEVELOPMENT"
	}
	fmt.Println("version:", ReleaseVer)
	fmt.Println("date:", ReleaseDate)
	fmt.Println("commit:", GitCommit)
}
