package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/google/uuid"
)

func main() {
	image := flag.String("docker-image", "", "A name of a Docker image")
	bashCommand := flag.String("bash-command", "", "A bash command (to run inside this Docker image)")
	flag.Parse()

	if *image == "" {
		panic("Cannot start without a docker image name")
	}

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		exit := make(chan os.Signal, 1)
		signal.Notify(exit, os.Interrupt, syscall.SIGTERM)
		<-exit
		cancel()
	}()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := Start(ctx, *image, []string{*bashCommand})
		if err != nil {
			fmt.Println(err)
		}
	}()

	wg.Wait()
	fmt.Println("Main done")
}

func Start(ctx context.Context, imageName string, bashCommand []string) (err error) {
	var entrypoint = []string{"/bin/bash", "-c"}
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return err
	}

	config := &types.ContainerCreateConfig{
		Name: uuid.NewString(),
		Config: &container.Config{
			Image:      imageName,
			Cmd:        bashCommand,
			Entrypoint: entrypoint,
			Tty:        true,
		},
	}

	result, err := cli.ImageSearch(ctx, config.Config.Image, types.ImageSearchOptions{
		Limit: 1,
	})
	if err != nil {
		return err
	}

	if len(result) == 0 {
		return errors.New(fmt.Sprintf("image with name: %s not found", config.Config.Image))
	}

	_, err = cli.ImagePull(ctx, config.Config.Image, types.ImagePullOptions{})
	if err != nil {
		return err
	}

	created, err := cli.ContainerCreate(ctx, config.Config, config.HostConfig, config.NetworkingConfig, config.Platform, config.Name)
	if err != nil {
		return err
	}

	err = cli.ContainerStart(ctx, created.ID, types.ContainerStartOptions{})
	if err != nil {
		return err
	}

	statusCh, errCh := cli.ContainerWait(ctx, created.ID, container.WaitConditionRemoved)

	select {
	case err = <-errCh:
	case <-statusCh:
		break
	default:
		out, err := cli.ContainerLogs(ctx, created.ID, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Follow: true})
		if err != nil {
			break
		}

		_, err = io.Copy(os.Stdout, out)
		if err != nil {
			break
		}

		time.Sleep(5 * time.Second)
	}

	err = cli.ContainerStop(context.Background(), created.ID, nil)
	if err != nil {
		return
	}

	err = cli.ContainerRemove(context.Background(), created.ID, types.ContainerRemoveOptions{Force: true})

	return
}
