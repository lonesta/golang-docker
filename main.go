package main

import (
	"context"
	"flag"
	"fmt"
	"golang-docker/docker"
	"os"
	"os/signal"

	"github.com/docker/docker/client"
)

func main() {
	imageName := flag.String("docker-image", "", "A name of a Docker image")
	bashCommand := flag.String("bash-command", "", "A bash command (to run inside this Docker image)")
	cloudWatchGroupName := flag.String("cloudwatch-group", "", "A name of an AWS CloudWatch group")
	cloudWatchStreamName := flag.String("cloudwatch-stram", "", "A name of an AWS CloudWatch stream")
	cloudWatchRegion := flag.String("aws-region", "", "A name of an AWS region")
	cloudWatchAccessKeyID := flag.String("aws-access-key-id", "", "Access key id AWS")
	cloudWatchSecretAccessKey := flag.String("aws-secret-access-key", "", "Secret access key AWS")
	flag.Parse()

	//if *imageName == "" {
	if *imageName == "" || *cloudWatchGroupName == "" || *cloudWatchStreamName == "" ||
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
