package docker

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/google/uuid"
)

type Container struct {
	ctx           context.Context
	imageName     string
	bashCommand   string
	containerName string
	containerID   string
	cli           *client.Client
}

func NewContainer(ctx context.Context, imageName string, bashCommand string, client *client.Client) *Container {
	return &Container{ctx: ctx, imageName: imageName, bashCommand: bashCommand, cli: client, containerName: uuid.NewString()}
}

//Create container from image with bash command and the random uuid name
func (c *Container) Create() error {
	searchResult, err := c.cli.ImageSearch(c.ctx, c.imageName, types.ImageSearchOptions{
		Limit: 1,
	})
	if err != nil {
		return err
	}

	if len(searchResult) == 0 {
		return fmt.Errorf("image with name: %s not found", c.imageName)
	}

	reader, err := c.cli.ImagePull(c.ctx, c.imageName, types.ImagePullOptions{})
	if err != nil {
		return err
	}

	defer reader.Close()
	_, err = io.Copy(os.Stdout, reader)
	if err != nil {
		return err
	}

	config := &types.ContainerCreateConfig{
		Name: c.containerName,
		Config: &container.Config{
			Image:      c.imageName,
			Cmd:        []string{c.bashCommand},
			Entrypoint: []string{"/bin/bash", "-c"},
			Tty:        true,
		},
	}

	created, err := c.cli.ContainerCreate(c.ctx, config.Config, config.HostConfig, config.NetworkingConfig, config.Platform, config.Name)
	if err != nil {
		_ = c.cli.ContainerRemove(context.TODO(), created.ID, types.ContainerRemoveOptions{Force: true})
		return err
	}

	c.containerID = created.ID

	return nil
}

// Start container
// Context cancellation stop and remove container
func (c *Container) Start() error {
	err := c.cli.ContainerStart(c.ctx, c.containerID, types.ContainerStartOptions{})
	if err != nil {
		_ = c.cli.ContainerRemove(context.TODO(), c.containerID, types.ContainerRemoveOptions{Force: true})
		return err
	}

	return nil
}

//Events running a logs listener from a container
func (c *Container) Events() (<-chan string, <-chan struct{}) {
	logs := make(chan string)
	done := make(chan struct{}, 1)
	waitConditionNotRunning, errChan := c.cli.ContainerWait(context.TODO(), c.containerID, container.WaitConditionNotRunning)

	go func() {
		out, err := c.cli.ContainerLogs(c.ctx, c.containerID, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Follow: true})
		if err != nil {
			logs <- err.Error()
		}
		defer out.Close()

		go func() {
			scan := bufio.NewScanner(out)
			for scan.Scan() {
				s := scan.Text()
				logs <- s
			}
		}()

		for {
			select {
			case err = <-errChan:
				logs <- err.Error()
				done <- struct{}{}
				return
			case status := <-waitConditionNotRunning:
				_ = c.cli.ContainerRemove(context.TODO(), c.containerID, types.ContainerRemoveOptions{Force: true})
				switch status.StatusCode {
				case 137:
					err = fmt.Errorf("container id: %s stopped manually", c.containerID)
				case 127:
					err = fmt.Errorf("cannot start: %s", c.containerID)
				default:
					err = fmt.Errorf("container id: %s stopped with %d exit code", c.containerID, status.StatusCode)
				}

				logs <- err.Error()
				done <- struct{}{}
				return
			case <-c.ctx.Done():
				err = c.cli.ContainerStop(context.TODO(), c.containerID, nil)
				if err != nil {
					logs <- err.Error()
					done <- struct{}{}
					return
				}

				err = c.cli.ContainerRemove(context.TODO(), c.containerID, types.ContainerRemoveOptions{Force: true})
				if err != nil {
					logs <- err.Error()
				}

				done <- struct{}{}
				return
			}
		}
	}()

	return logs, done
}
