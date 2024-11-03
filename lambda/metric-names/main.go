package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	v4 "github.com/aws/aws-sdk-go/aws/signer/v4"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type Config struct {
	AMPQueryURL string
	queueURL    string
	region      string
	sess        *session.Session
	log         *zap.SugaredLogger
}

type metricNames struct {
	Names  []string `json:"data"`
	Status string   `json:"status"`
}

type metricCardinality struct {
	Name              string `json:"name"`
	TotalMetricsCount int    `json:"totalMetricsCount"`
}

func newConfig(log *zap.SugaredLogger) *Config {
	// Need AWS session to sign queries to Lambda Function URLs
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))

	workspaceId := os.Getenv("AMP_WORKSPACE_ID")
	region := os.Getenv("AWS_REGION")
	sqsQueueURL := os.Getenv("SQS_QUEUE_URL")

	return &Config{
		AMPQueryURL: fmt.Sprintf("https://aps-workspaces.%s.amazonaws.com/workspaces/%s/api/v1/label/__name__/values", region, workspaceId),
		queueURL:    sqsQueueURL,
		region:      region,
		sess:        sess,
		log:         log,
	}

}

func handler(ctx context.Context) error {

	logger, _ := zap.NewProduction()
	defer logger.Sync()
	log := logger.Sugar()

	cfg := newConfig(log)
	names, err := getMetricNames(cfg)
	cfg.log.Infow("",
		"metric_names", len(names.Names),
	)

	jobs := splitJobs(names.Names)
	cfg.log.Infow("",
		"batches", len(jobs),
	)

	for _, batch := range jobs {
		err := sqsEnqueue(cfg, batch, len(names.Names))
		logError(err, "failed enqueueing", cfg.log)
	}

	return err
}

func splitJobs(jobs []string) [][]string {
	inputSize := len(jobs)
	if inputSize == 0 {
		return [][]string{}
	}
	chunksMaxSize := 10 // sqs message batch size

	if inputSize <= chunksMaxSize {
		return [][]string{jobs}
	}
	res := [][]string{}

	count := 0
	buffer := []string{}
	// chunkIndex := 0
	for i, job := range jobs {

		buffer = append(buffer, job)
		count++

		// last chunk less than 10
		if i == inputSize-1 && inputSize%chunksMaxSize != 0 {
			res = append(res, buffer)
		}

		if count == chunksMaxSize {
			res = append(res, buffer)
			buffer = []string{}
			count = 0
		}
	}

	return res
}

func getMetricNames(cfg *Config) (metricNames, error) {
	queryRange := unixEpochFloat()

	url := fmt.Sprintf("%s?end=%f&start=%f", cfg.AMPQueryURL, queryRange, queryRange)
	cfg.log.Info(url)

	metricsRes, err := signedQuery(cfg, "GET", url, nil)
	if err != nil {
		logError(err, "query error", cfg.log)
		return metricNames{}, err
	}

	var res metricNames
	err = json.NewDecoder(metricsRes.Body).Decode(&res)
	if err != nil {
		logError(err, "query error", cfg.log)
		return metricNames{}, err
	}

	return res, err
}

func sqsEnqueue(cfg *Config, names []string, totalMetricsCount int) error {

	sqsClient := sqs.New(cfg.sess)
	params := &sqs.SendMessageBatchInput{
		Entries: func() []*sqs.SendMessageBatchRequestEntry {
			res := []*sqs.SendMessageBatchRequestEntry{}
			for _, name := range names {
				job := metricCardinality{
					Name:              name,
					TotalMetricsCount: totalMetricsCount,
				}
				metricCardinalityBytes, err := json.Marshal(job)
				if err != nil {
					logError(err, "failed marshalling", cfg.log)
					return res
				}
				res = append(res, &sqs.SendMessageBatchRequestEntry{
					Id:          aws.String(uuid.NewString()),
					MessageBody: aws.String(string(metricCardinalityBytes)),
				})
			}
			return res
		}(),
		QueueUrl: &cfg.queueURL,
	}
	_, err := sqsClient.SendMessageBatch(params)
	logError(err, "failed sending message", cfg.log)
	return err
}

func signedQuery(cfg *Config, method string, url string, data io.ReadCloser) (*http.Response, error) {
	request, err := http.NewRequest(method, url, data)
	logError(err, "failed creating request", cfg.log)

	// need fresh credentials as AWS session token can expired
	credsValue, err := cfg.sess.Config.Credentials.Get()
	logError(err, "failed creating session", cfg.log)

	credentials := credentials.NewStaticCredentialsFromCreds(credsValue)
	signer := v4.NewSigner(credentials)

	if data != nil {
		b, _ := io.ReadAll(data)
		signer.Sign(request, strings.NewReader(string(b)), "aps", cfg.region, time.Now())
	} else {
		signer.Sign(request, nil, "aps", cfg.region, time.Now())
	}

	client := http.Client{
		Timeout: 5 * time.Second,
	}
	return client.Do(request)
}

func logError(err error, message string, log *zap.SugaredLogger) {
	if err != nil {
		log.Errorw(message,
			"err", err,
		)
	}
}

// unixEpochFloat gives the current time in seconds since epoch
func unixEpochFloat() float64 {
	t := time.Now()
	nanos := t.UnixNano()
	queryRange := float64(nanos) / float64(time.Second)
	return queryRange - 20 // moving the query range a bit in the past to ensure having data
}

func main() {
	lambda.Start(handler)
}
