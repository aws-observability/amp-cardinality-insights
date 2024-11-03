package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
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

/*
	{
	   "status":"success",
	   "data":{
	      "resultType":"vector",
	      "result":[
	         {
	            "metric":{
	               "__name__":"apiserver_request_duration_seconds_bucket"
	            },
	            "value":[
	               1690841070.445,
	               "6710"
	            ]
	         }
	      ]
	   }
	}
*/
type PromQLResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric struct {
				Metric map[string]string `json:"metric"`
			}
			Value []interface{} `json:"value"`
		}
	}
}

type metricCardinality struct {
	Name              string `json:"name"`
	Count             int    `json:"count,omitempty"`
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
		AMPQueryURL: fmt.Sprintf("https://aps-workspaces.%s.amazonaws.com/workspaces/%s/api/v1/query", region, workspaceId),
		queueURL:    sqsQueueURL,
		region:      region,
		sess:        sess,
		log:         log,
	}

}

func handler(ctx context.Context, sqsEvent events.SQSEvent) error {

	logger, _ := zap.NewProduction()
	defer logger.Sync()
	log := logger.Sugar()

	cfg := newConfig(log)

	batch := []metricCardinality{}

	for _, message := range sqsEvent.Records {
		var input metricCardinality
		err := json.Unmarshal([]byte(message.Body), &input)

		if err != nil {
			log.Error("failed unmarshalling", err)
			continue
		}

		cardinality, err := getMetricCardinality(cfg, input.Name)
		log.Info("cardinality", cardinality)
		logError(err, "query error", cfg.log)
		if cardinality > 0 {
			batch = append(batch, metricCardinality{
				Name:              input.Name,
				Count:             cardinality,
				TotalMetricsCount: input.TotalMetricsCount,
			})
		}

	}

	err := sqsEnqueue(cfg, batch)
	if err != nil {
		logError(err, "failed queueing", cfg.log)
		return err
	}
	log.Infof("enqueued %d jobs", len(batch))
	return err
}

// AMP query
func getMetricCardinality(cfg *Config, name string) (int, error) {
	query := url.QueryEscape(fmt.Sprintf("count by (__name__) ({__name__=\"%s\"})", name))
	queryRange := unixEpochFloat()
	queryUrl := fmt.Sprintf("%s?end=%f&start=%f&query=%s", cfg.AMPQueryURL, queryRange, queryRange, query)
	cfg.log.Info(queryUrl)

	metricsRes, err := signedQuery(cfg, "GET", queryUrl, nil)
	if err != nil {
		logError(err, "query error", cfg.log)
		return 0, err
	}

	b, _ := io.ReadAll(metricsRes.Body)
	var res PromQLResponse
	err = json.Unmarshal(b, &res)
	if err != nil {
		logError(err, "failed unmarshalling", cfg.log)
		cfg.log.Info("raw response ", string(b))
		return 0, err
	}

	cfg.log.Info("raw response ", string(b))
	cfg.log.Info("parsed response ", res)

	if res.Status == "success" {
		if len(res.Data.Result) == 0 {
			return 0, fmt.Errorf("empty result for %s", name)
		}

		if len(res.Data.Result[0].Value) < 2 {
			return 0, fmt.Errorf("empty result for %s", name)
		}

		card, err := strconv.Atoi(res.Data.Result[0].Value[1].(string))
		return card, err

	}

	return 0, fmt.Errorf("failed getting cardinality for %s", name)
}

func sqsEnqueue(cfg *Config, jobs []metricCardinality) error {

	if len(jobs) == 0 {
		return nil
	}

	sqsClient := sqs.New(cfg.sess)
	params := &sqs.SendMessageBatchInput{
		Entries: func() []*sqs.SendMessageBatchRequestEntry {
			res := []*sqs.SendMessageBatchRequestEntry{}
			for _, job := range jobs {
				metricCardinalityBytes, err := json.Marshal(job)
				logError(err, "failed marshalling", cfg.log)
				if err == nil {
					res = append(res, &sqs.SendMessageBatchRequestEntry{
						Id:          aws.String(uuid.NewString()),
						MessageBody: aws.String(string(metricCardinalityBytes)),
					})
				}
			}
			return res
		}(),
		QueueUrl: &cfg.queueURL,
	}
	r, err := sqsClient.SendMessageBatch(params)
	cfg.log.Info(r, err)
	return err
}

func signedQuery(cfg *Config, method string, url string, data io.ReadCloser) (*http.Response, error) {
	request, err := http.NewRequest(method, url, data)
	logError(err, "failed creating request", cfg.log)

	// need fresh credentials as AWS session token can expire
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
		Timeout: 30 * time.Second,
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
