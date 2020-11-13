package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"

	"os"

	log "github.com/sirupsen/logrus"
)

func (t *Twitter) UploadVideo(file string, status string) error {
	if file == "" {
		return errors.New("file to upload is required. Use the '-f' flag to set a file to upload.")
	}

	media, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}

	err = t.UpdateStatusWithVideo(status, media)
	if err != nil {
		return err
	}

	return nil
}

func main() {
	consumerKey := os.Getenv("TWITTER_CONSUMER_KEY")
	consumerSecret := os.Getenv("TWITTER_CONSUMER_SECRET")
	accessToken := os.Getenv("TWITTER_ACCESS_TOKEN")
	accessSecret := os.Getenv("TWITTER_ACCESS_SECRET")

	uploadCmd := flag.NewFlagSet("upload", flag.ExitOnError)
	status := uploadCmd.String("s", "", "Status to tweet")
	debugMode := uploadCmd.Bool("d", false, "Enable debug logging")

	if consumerKey == "" || consumerSecret == "" || accessToken == "" || accessSecret == "" {
		fmt.Println("Consumer key/secret and Access token/secret required. These must be saved as environment variables.")
		os.Exit(1)
	}

	if len(os.Args) < 2 {
		fmt.Println("expected 'upload' subcommand")
		os.Exit(1)
	}

	client, err := NewTwitter(consumerKey, consumerSecret, accessToken, accessSecret)
	if err != nil {
		log.Error(err)
	}

	switch os.Args[1] {
	case "upload":
		uploadCmd.Parse(os.Args[2:])
		if *debugMode {
			log.SetLevel(log.DebugLevel)
		} else {
			log.SetLevel(log.ErrorLevel)
		}
		args := uploadCmd.Args()
		if len(args) != 1 {
			log.Error("please include a video to upload")
			os.Exit(1)
		}
		err := client.UploadVideo(args[0], *status)
		if err != nil {
			fmt.Println("Upload Failure:", err)
		}
		os.Exit(0)
	default:
		fmt.Println("expected 'upload' subcommand")
		os.Exit(1)
	}
}
