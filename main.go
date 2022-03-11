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
	flag.Parse()

	if *imageName == "" {
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
