package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"time"
)

type function struct {
	awsClient            *lambda.Client
	arn                  *string
	aliasName            *string
	currentNewVersionPct float64
	newVersion           *string
	step                 *float64
}

func main() {
	arn := flag.String("arn", "", "Specify the arn of the lambda function to deploy")
	alias := flag.String("alias", "live", "Set the alias name you want to deploy against")
	target := flag.String("target", "", "Set the target version you want to deploy the alias towards")
	step := flag.Float64("step", 0.1, "A value between 0.00 and 1.0 counting the step for the deployment")
	flag.Parse()
	f, err := NewFunction(
		arn,
		alias,
		target,
		step,
	)
	if err != nil {
		panic(err)
	}
	if err := f.deploy(context.Background()); err != nil {
		panic(err)
	}
}

func NewFunction(arn, aliasName, newVersion *string, step *float64) (*function, error) {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, err
	}
	return &function{
		awsClient:            lambda.NewFromConfig(cfg),
		arn:                  arn,
		aliasName:            aliasName,
		currentNewVersionPct: 0,
		newVersion:           newVersion,
		step:                 step,
	}, nil
}

func (f *function) deploy(ctx context.Context) error {
	ticker := time.NewTicker(time.Duration(time.Second))
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return errors.New("context cancelled")
		case <-ticker.C:
			if f.currentNewVersionPct >= 1-*f.step {
				if err := f.promoteNewVersion(); err != nil {
					return err
				}
				return nil
			}
			if err := f.adjustTrafficSplit(); err != nil {
				return err
			}
			fmt.Printf("Current newVersionPct: %f\n", f.currentNewVersionPct)
		}
	}
}

func (f *function) adjustTrafficSplit() error {
	_, err := f.awsClient.UpdateAlias(context.Background(), &lambda.UpdateAliasInput{
		FunctionName: f.arn,
		Name:         f.aliasName,
		RoutingConfig: &types.AliasRoutingConfiguration{
			AdditionalVersionWeights: map[string]float64{*f.newVersion: f.currentNewVersionPct + *f.step}},
	})
	if err != nil {
		return err
	}
	f.currentNewVersionPct += *f.step
	return nil
}

func (f *function) promoteNewVersion() error {
	_, err := f.awsClient.UpdateAlias(context.Background(), &lambda.UpdateAliasInput{
		FunctionName:    f.arn,
		Name:            f.aliasName,
		FunctionVersion: f.newVersion,
		RoutingConfig:   &types.AliasRoutingConfiguration{},
	})
	if err != nil {
		return err
	}
	return nil
}
