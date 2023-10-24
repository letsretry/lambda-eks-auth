package main

import (
	"encoding/base64"
	"log"
	"fmt"
	"os"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/eks"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/aws-iam-authenticator/pkg/token"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Event struct {
	KubernetesName string `json:"kubernetesName"`
	Region         string `json:"region"`
	LogLevel       string `json:"logLevel"`
	
}
func getLogger(loglevel *string) *zap.SugaredLogger {
	atom := zap.NewAtomicLevel()

	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "timestamp"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	logger := zap.New(zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderCfg),
		zapcore.Lock(os.Stdout),
		atom,
	))

	defer logger.Sync()

	switch *loglevel {
	case "info":
		fmt.Println("logLevel : info")
		atom.SetLevel(zap.InfoLevel)
	case "warn":
		fmt.Println("logLevel : warn")
		atom.SetLevel(zap.WarnLevel)
	case "error":
		fmt.Println("logLevel : error")
		atom.SetLevel(zap.ErrorLevel)
	case "debug":
		fmt.Println("logLevel : debug")
		atom.SetLevel(zap.DebugLevel)
	}

	sugar := logger.Sugar()
	return sugar
}

func newClientset(cluster *eks.Cluster, sugar *zap.SugaredLogger) (*kubernetes.Clientset, error) {

	gen, err := token.NewGenerator(true, false)
	if err != nil {
		return nil, err
	}
	opts := &token.GetTokenOptions{
		ClusterID: aws.StringValue(cluster.Name),
	}
	tok, err := gen.GetWithOptions(opts)
	if err != nil {
		return nil, err
	}
	ca, err := base64.StdEncoding.DecodeString(aws.StringValue(cluster.CertificateAuthority.Data))
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(
		&rest.Config{
			Host:        aws.StringValue(cluster.Endpoint),
			BearerToken: tok.Token,
			TLSClientConfig: rest.TLSClientConfig{
				CAData: ca,
			},
		},
	)

	if err != nil {
		return nil, err
	}
	return clientset, nil
}

func LambdaHandler(e Event) (string, error) {
	sugar := getLogger(&e.LogLevel)

	log.Printf("%T", &e)

	if e.KubernetesName == "" {
		sugar.Error("Empty Cluster name in event")
		return "empty Kubernetes Name", nil
	}
	if e.LogLevel == "" {
		sugar.Warn("No Log Level Set.Using default info")
		e.LogLevel = "info"
	}

	if e.Region == "" {
		sugar.Warn("No Region Set.Using default eu-west-1")
		e.Region = "eu-west-1"
	}

	sugar.Info("Region :", e.Region)
	sugar.Infof("LogLevel : %+v", e.LogLevel)
	sugar.Infof("EKS Cluster Name : %+v", e.KubernetesName)

	sugar.Debug("Getting Session for region", e.Region)
	sess := session.Must(session.NewSession(&aws.Config{
		Region: aws.String(e.Region),
	}))
	eksSvc := eks.New(sess)

	sugar.Infof("Getting Cluster Details for :%+v", e.KubernetesName)
	input := &eks.DescribeClusterInput{
		Name: aws.String(e.KubernetesName),
	}

	result, err := eksSvc.DescribeCluster(input)
	if err != nil {
		return "Error Describing Clusters", err
	}
	sugar.Debug("Describing Cluster %+v", result)

	clientset, err := newClientset(result.Cluster, sugar)
	if err != nil {
		return "Error creating clientset", err
	}
	sugar.Debug("Client Set :", clientset)

	return "Success", err

}

func main() {
	lambda.Start(LambdaHandler)
}
