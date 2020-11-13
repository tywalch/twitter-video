package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mrjones/oauth"
	log "github.com/sirupsen/logrus"
)

const (
	statusUpdate      string = "https://api.twitter.com/1.1/statuses/update.json"
	mediaUpload       string = "https://upload.twitter.com/1.1/media/upload.json"
	requestTokenURL   string = "https://api.twitter.com/oauth/request_token"
	authorizeTokenURL string = "https://api.twitter.com/oauth/authorize"
	accessTokenURL    string = "https://api.twitter.com/oauth/access_token"
)

// Twitter is an httpClient established with twitter oauth configuration
type Twitter struct {
	client *http.Client
}

type mediaInitResponse struct {
	MediaID          uint64 `json:"media_id"`
	MediaIDString    string `json:"media_id_string"`
	ExpiresAfterSecs uint64 `json:"expires_after_secs"`
}

type mediaFinalProcessingInfo struct {
	CheckAfterSeconds int    `json:"check_after_secs"`
	State             string `json:"state"`
}

type mediaStatusResponse struct {
	MediaID          uint64                   `json:"media_id"`
	MediaIDString    string                   `json:"media_id_string"`
	MediaKey         string                   `json:"media_key"`
	Size             int                      `json:"size"`
	ExpiresAfterSecs uint64                   `json:"expires_after_secs"`
	ProcessingInfo   mediaFinalProcessingInfo `json:"processing_info"`
}

// NewTwitter creates and configures an oauth consumer for an httpClient
func NewTwitter(consumerKey, consumerSecret, accessToken, accessSecret string) (*Twitter, error) {
	consumer := oauth.NewConsumer(
		consumerKey,
		consumerSecret,
		oauth.ServiceProvider{
			RequestTokenUrl:   requestTokenURL,
			AuthorizeTokenUrl: authorizeTokenURL,
			AccessTokenUrl:    accessTokenURL,
		})

	authentication := &oauth.AccessToken{
		Token:  accessToken,
		Secret: accessSecret,
	}

	client, err := consumer.MakeHttpClient(authentication)
	if err != nil {
		return nil, err
	}

	return &Twitter{client}, nil
}

// UpdateStatusWithVideo uploads a video to twitter and creates new post with video file
func (twitter *Twitter) UpdateStatusWithVideo(status string, media []byte) error {
	log.Debug("bytes", len(media))

	fmt.Print("Initializing... ")
	mediaID, err := twitter.mediaInit(media)
	if err != nil {
		log.Debug("Can't init media", err)
		return err
	}

	fmt.Print("Uploading... ")
	if err := twitter.mediaAppend(mediaID, media); err != nil {
		log.Debug("Cant't append media", err)
		return err
	}

	fmt.Print("Finalizing... ")
	if err := twitter.mediaFinilize(mediaID); err != nil {
		log.Debug("Cant't append media", err)
		return err
	}

	fmt.Print("Posting... ")
	if twitter.updateStatusWithMediaID(status, mediaID) != nil {
		log.Debug("Can't update status", err)
		return err
	}
	fmt.Println("Upload Complete!")

	return nil
}

func sleep(seconds int) {
	time.Sleep(time.Duration(seconds*2) * time.Second)
}

func (twitter *Twitter) updateStatusWithMediaID(text string, mediaID uint64) error {
	form := url.Values{}
	form.Add("status", text)
	form.Add("media_ids", fmt.Sprint(mediaID))

	req, err := http.NewRequest("POST", statusUpdate, strings.NewReader(form.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	res, err := twitter.client.Do(req)
	if err != nil {
		return err
	}

	body, err := ioutil.ReadAll(res.Body)
	log.Debug("update status response ", string(body))

	return nil
}

func (twitter *Twitter) mediaAppend(mediaID uint64, media []byte) error {
	step := 500 * 1024
	for s := 0; s*step < len(media); s++ {
		var body bytes.Buffer
		rangeBegining := s * step
		rangeEnd := (s + 1) * step
		if rangeEnd > len(media) {
			rangeEnd = len(media)
		}

		log.Debug("try to append ", rangeBegining, "-", rangeEnd)

		w := multipart.NewWriter(&body)

		w.WriteField("command", "APPEND")
		w.WriteField("media_id", fmt.Sprint(mediaID))
		w.WriteField("segment_index", fmt.Sprint(s))

		fw, err := w.CreateFormFile("media", "out.mp4")
		if err != nil {
			return err
		}

		log.Debug(body.String())

		n, err := fw.Write(media[rangeBegining:rangeEnd])
		if err != nil {
			return err
		}

		log.Debug("len ", n)

		w.Close()

		req, err := http.NewRequest("POST", mediaUpload, &body)
		if err != nil {
			return err
		}

		req.Header.Add("Content-Type", w.FormDataContentType())

		res, err := twitter.client.Do(req)
		if err != nil {
			return err
		}

		resBody, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return err
		}

		log.Debug("append response ", string(resBody))
	}

	return nil
}

func (twitter *Twitter) mediaInit(media []byte) (uint64, error) {
	var initResponse mediaInitResponse
	var mediaID uint64

	form := url.Values{}
	form.Add("command", "INIT")
	form.Add("media_type", "video/mp4")
	form.Add("check_progress", "True")
	form.Add("media_category", "amplify_video")
	form.Add("total_bytes", fmt.Sprint(len(media)))

	req, err := http.NewRequest("POST", mediaUpload, strings.NewReader(form.Encode()))
	if err != nil {
		return mediaID, err
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	res, err := twitter.client.Do(req)
	if err != nil {
		return mediaID, err
	}

	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return mediaID, err
	}

	log.Debug("response", string(body))

	err = json.Unmarshal(body, &initResponse)
	if err != nil {
		return mediaID, err
	}

	log.Debug("Initialized media: ", initResponse)
	mediaID = initResponse.MediaID

	return mediaID, nil
}

func (twitter *Twitter) checkStatus(mediaID uint64, state string, checkAfterSeconds int, attempts int) error {
	if state == "succeeded" {
		return nil
	} else if state == "failed" {
		return errors.New("Failed response")
	} else if attempts > 3 {
		return errors.New("Max attempts reached")
	} else {
		attempts++
	}

	sleep(checkAfterSeconds)

	req, err := http.NewRequest("GET", mediaUpload, nil)
	if err != nil {
		return err
	}

	q := req.URL.Query()
	q.Add("command", "STATUS")
	q.Add("media_id", fmt.Sprint(mediaID))
	req.URL.RawQuery = q.Encode()

	res, err := twitter.client.Do(req)
	if err != nil {
		return err
	}

	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}

	log.Debug("status", string(body))

	var statusCheck mediaStatusResponse
	err = json.Unmarshal(body, &statusCheck)
	if err != nil {
		return err
	} else if statusCheck.ProcessingInfo.State == "" {
		return errors.New(string(body))
	} else {
		return twitter.checkStatus(mediaID, statusCheck.ProcessingInfo.State, statusCheck.ProcessingInfo.CheckAfterSeconds, attempts)
	}
}

func (twitter *Twitter) mediaFinilize(mediaID uint64) error {
	form := url.Values{}
	form.Add("command", "FINALIZE")
	form.Add("media_id", fmt.Sprint(mediaID))

	req, err := http.NewRequest("POST", mediaUpload, strings.NewReader(form.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	res, err := twitter.client.Do(req)
	if err != nil {
		return err
	}

	body, err := ioutil.ReadAll(res.Body)
	log.Debug("final response ", string(body))

	var statusResponse mediaStatusResponse
	err = json.Unmarshal(body, &statusResponse)
	if err != nil {
		return err
	}

	return twitter.checkStatus(mediaID, statusResponse.ProcessingInfo.State, statusResponse.ProcessingInfo.CheckAfterSeconds, 0)
}
