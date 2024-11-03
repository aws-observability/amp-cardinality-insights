package main

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"strconv"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type Config struct {
	ingestQueueURL string
	region         string
	topNCardValue  string
	sess           *session.Session
	log            *zap.SugaredLogger
}

type metricCardinality struct {
	Name              string `json:"name"`
	Count             int    `json:"count"`
	TotalMetricsCount int    `json:"totalMetricsCount"`
}

type Cardinality struct {
	Key   string
	Value int
}

type CardinalityList []Cardinality

func (p CardinalityList) Len() int           { return len(p) }
func (p CardinalityList) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p CardinalityList) Less(i, j int) bool { return p[i].Value < p[j].Value }

func newConfig(log *zap.SugaredLogger) *Config {
	// Need AWS session to sign queries to Lambda Function URLs
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))

	return &Config{
		ingestQueueURL: os.Getenv("INGEST_QUEUE_URL"),
		region:         os.Getenv("AWS_REGION"),
		topNCardValue:  os.Getenv("TOPN_CARDINALITY_VALUE"),
		sess:           sess,
		log:            log,
	}

}

func main() {
	lambda.Start(handler)
}

func handler(ctx context.Context, sqsEvent events.SQSEvent) error {

	logger, _ := zap.NewProduction()
	defer logger.Sync()
	log := logger.Sugar()
	cfg := newConfig(log)

	cardList := make(map[string]int, 0)

	var totalMetricsCount int

	//deduplicate
	for i, message := range sqsEvent.Records {
		var c metricCardinality
		err := json.Unmarshal([]byte(message.Body), &c)
		logError(err, "failed unmarshalling", log)
		if err == nil {
			cardList[c.Name] = c.Count
			if i == len(sqsEvent.Records)-1 {
				totalMetricsCount = c.TotalMetricsCount
			}
		}
	}

	functionReceivedMessages := len(cardList)
	// extract total
	log.Infow("stats",
		"totalMetricsCount", totalMetricsCount,
		"functionReceivedMessages", functionReceivedMessages,
	)

	// if functionReceivedMessages >= totalMetricsCount {
	// If the function has gathered all metrics from the first phase
	// we can do a topN and enqueue for ingestion into AMP

	topNCardValue, err := strconv.Atoi(cfg.topNCardValue)
	logError(err, "failed parsing topNCardValue, using top(10)", log)
	if err != nil {
		topNCardValue = 10
	}

	res := topN(cardList, topNCardValue) // N should be a param by the customer at least default
	log.Info("enqueue for ingestion into AMP", res)
	sqsEnqueue(cfg, cfg.ingestQueueURL, res, totalMetricsCount)

	return nil
}

func sqsEnqueue(cfg *Config, queueURL string, jobs CardinalityList, totalMetricsCount int) error {
	cfg.log.Infof("received %d jobs to enqueue", len(jobs))

	if len(jobs) == 0 {
		return nil
	}

	sqsClient := sqs.New(cfg.sess)
	params := &sqs.SendMessageBatchInput{
		Entries: func() []*sqs.SendMessageBatchRequestEntry {
			res := []*sqs.SendMessageBatchRequestEntry{}
			for _, job := range jobs {
				metricCardinalityBytes, err := json.Marshal(metricCardinality{
					Name:              job.Key,
					Count:             job.Value,
					TotalMetricsCount: totalMetricsCount,
				})
				logError(err, "failed marshalling", cfg.log)
				if err != nil {
					return res
				}
				res = append(res, &sqs.SendMessageBatchRequestEntry{
					Id:          aws.String(uuid.NewString()),
					MessageBody: aws.String(string(metricCardinalityBytes)),
				})
			}
			return res
		}(),
		QueueUrl: &queueURL,
	}
	res, err := sqsClient.SendMessageBatch(params)
	cfg.log.Info(res, err)
	return err
}

func topN(list map[string]int, n int) CardinalityList {

	if len(list) == 0 {
		return CardinalityList{}
	}

	i := 0
	p := make(CardinalityList, len(list))
	for k, v := range list {
		p[i] = Cardinality{k, v}
		i++
	}
	sort.Sort(sort.Reverse(p))

	if len(p) <= n {
		return p
	}

	return p[0:n]
}

func logError(err error, message string, log *zap.SugaredLogger) {
	if err != nil {
		log.Errorw(message,
			"err", err,
		)
	}
}
