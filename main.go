package main

import (
	"context"
	"flag"
	"fmt"
	"golang-docker/docker"
	"os"
	"os/signal"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/docker/docker/client"
)

var cwl *cloudwatchlogs.CloudWatchLogs

func main() {
	imageName := flag.String("docker-image", "", "A name of a Docker image")
	bashCommand := flag.String("bash-command", "", "A bash command (to run inside this Docker image)")
	cloudWatchGroupName := flag.String("cloudwatch-group", "", "A name of an AWS CloudWatch group")
	cloudWatchStreamName := flag.String("cloudwatch-stream", "", "A name of an AWS CloudWatch stream")
	cloudWatchRegion := flag.String("aws-region", "", "A name of an AWS region")
	cloudWatchAccessKeyID := flag.String("aws-access-key-id", "", "Access key id AWS")
	cloudWatchSecretAccessKey := flag.String("aws-secret-access-key", "", "Secret access key AWS")
	flag.Parse()

	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
		Config: aws.Config{
			Region:      cloudWatchRegion,
			Credentials: credentials.NewStaticCredentials(*cloudWatchAccessKeyID, *cloudWatchSecretAccessKey, ""),
		},
	}))

	cwl = cloudwatchlogs.New(sess)
	err := ensureLogGroupExists(*cloudWatchGroupName)
	if err != nil {
		panic(err)
	}

	err = ensureLogStreamExists(*cloudWatchGroupName, *cloudWatchStreamName)
	if err != nil {
		panic(err)
	}

	if *imageName == "" ||
		*cloudWatchGroupName == "" || *cloudWatchStreamName == "" ||
		*cloudWatchRegion == "" || *cloudWatchAccessKeyID == "" || *cloudWatchSecretAccessKey == "" {
		fmt.Println("Cannot start without a docker image name")
		return
	}

	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		fmt.Println(err)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		exit := make(chan os.Signal, 1)
		signal.Notify(exit, os.Interrupt, os.Kill)
		<-exit
		cancel()
	}()

	ctn := docker.NewContainer(ctx, *imageName, *bashCommand, cli)
	err = ctn.Create()
	if err != nil {
		fmt.Println(err)
		return
	}

	err = ctn.Start()
	if err != nil {
		fmt.Println(err)
		return
	}

	logsChan, doneChan := ctn.Events()
	for {
		select {
		case <-doneChan:
			fmt.Println("FINISH")
			return
		case val := <-logsChan:
			fmt.Println(val)
		}
	}
}

func ensureLogGroupExists(name string) error {
	resp, err := cwl.DescribeLogGroups(&cloudwatchlogs.DescribeLogGroupsInput{})
	if err != nil {
		return err
	}

	for _, logGroup := range resp.LogGroups {
		if *logGroup.LogGroupName == name {
			return nil
		}
	}

	_, err = cwl.CreateLogGroup(&cloudwatchlogs.CreateLogGroupInput{
		LogGroupName: &name,
	})
	if err != nil {
		return err
	}

	_, err = cwl.PutRetentionPolicy(&cloudwatchlogs.PutRetentionPolicyInput{
		RetentionInDays: aws.Int64(1),
		LogGroupName:    &name,
	})

	return err
}

func ensureLogStreamExists(logGroupName, logStreamName string) error {
	resp, err := cwl.DescribeLogStreams(&cloudwatchlogs.DescribeLogStreamsInput{LogGroupName: aws.String(logGroupName)})
	if err != nil {
		return err
	}

	for _, logStream := range resp.LogStreams {
		if *logStream.LogStreamName == logStreamName {
			return nil
		}
	}

	_, err = cwl.CreateLogStream(&cloudwatchlogs.CreateLogStreamInput{
		LogGroupName:  aws.String(logGroupName),
		LogStreamName: aws.String(logStreamName),
	})
	if err != nil {
		return err
	}

	return err
}
