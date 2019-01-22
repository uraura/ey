package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/ssm"
)

type instanceIDs []*string

func (i *instanceIDs) String() string {
	return fmt.Sprintf("%v", *i)
}

func (i *instanceIDs) Set(v string) error {
	*i = append(*i, &v)
	return nil
}

func main() {
	var instanceIDs instanceIDs
	flag.Var(&instanceIDs, "i", "target EC2 instance id")

	s3bucket := flag.String("s3bucket", "", "temporary file storage")
	s3prefix := flag.String("s3prefix", "ey", "prefix for temporary files")
	dryrun := flag.Bool("dryrun", false, "dryrun")

	flag.Parse()
	srcdst := flag.Args()
	if len(srcdst) < 2 {
		// src and dst are required
		os.Exit(1)
	}
	dst := srcdst[len(srcdst)-1]
	srcs := srcdst[0:len(srcdst)-1]

	fmt.Println(*s3bucket, *s3prefix, *dryrun)
	fmt.Printf("%v %v", dst, srcs)

	// upload files to temporary S3 bucket
	session := session.Must(session.NewSession())
	s3session := s3.New(session)
	for _, src := range srcs {
		input := &s3.PutObjectInput{
			Body:   aws.ReadSeekCloser(strings.NewReader(src)),
			Bucket: aws.String(*s3bucket),
			Key:    aws.String(fmt.Sprintf("%s/%s", *s3prefix, filepath.Base(src))),
		}

		_, err := s3session.PutObject(input)
		if err != nil {
			if aerr, ok := err.(awserr.Error); ok {
				switch aerr.Code() {
				default:
					fmt.Println(aerr.Error())
					os.Exit(1)
				}
			} else {
				// Print the error, cast err to awserr.Error to get the Code and
				// Message from an error.
				fmt.Println(err.Error())
				os.Exit(1)
			}
		}

		// run SSM command
		ssmsession := ssm.New(session)
		input2 := &ssm.SendCommandInput{
			CloudWatchOutputConfig: nil,
			Comment:                nil,
			DocumentHash:           nil,
			DocumentHashType:       nil,
			DocumentName:           aws.String("AWS-RunShellScript"),
			DocumentVersion:        nil,
			InstanceIds:            instanceIDs,
			MaxConcurrency:         nil,
			MaxErrors:              nil,
			NotificationConfig:     nil,
			OutputS3BucketName:     nil,
			OutputS3KeyPrefix:      nil,
			OutputS3Region:         nil,
			Parameters:             map[string][]*string{
				"commands": {aws.String(fmt.Sprintf(`aws s3 cp s3://%s/%s/%s %s --dryrun`, *s3bucket, *s3prefix, src, dst))},
				"executionTimeout": {aws.String("60")},
			},
			ServiceRoleArn:         nil,
			Targets:                nil,
			TimeoutSeconds:         nil,
		}
		result, err := ssmsession.SendCommand(input2)
		if err != nil {
			if aerr, ok := err.(awserr.Error); ok {
				switch aerr.Code() {
				default:
					fmt.Println(aerr.Error())
					os.Exit(1)
				}
			} else {
				// Print the error, cast err to awserr.Error to get the Code and
				// Message from an error.
				fmt.Println(err.Error())
				os.Exit(1)
			}
		}

		var instanceIDs2 []*string
		copy(instanceIDs2, instanceIDs)
	CheckRunCommandResult:
		for i, id := range instanceIDs2 {
			result2, err := ssmsession.GetCommandInvocation(&ssm.GetCommandInvocationInput{
				CommandId:  result.Command.CommandId,
				InstanceId: id,
			})
			if err != nil {
				if aerr, ok := err.(awserr.Error); ok {
					switch aerr.Code() {
					default:
						fmt.Println(aerr.Error())
						os.Exit(1)
					}
				} else {
					// Print the error, cast err to awserr.Error to get the Code and
					// Message from an error.
					fmt.Println(err.Error())
					os.Exit(1)
				}
			}

			time.Sleep(3 * time.Second)

			if *result2.Status == ssm.CommandInvocationStatusSuccess {
				fmt.Println(id)
				fmt.Println(*result2.StandardOutputContent)
				instanceIDs2 = append(instanceIDs2[:i], instanceIDs2[i+1:]...)
				goto CheckRunCommandResult
			}
		}
	}

}
