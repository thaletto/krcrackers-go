# Deployment (AWS Lambda)

## Architecture

```
Client → API Gateway v2 (HTTP API) → Lambda → D1 (via HTTPS)
```

Lambda runs your Go binary as a Linux process. D1 is accessed over HTTPS using the existing `cloudflare-go/v7` SDK — no changes to database code.

## Prerequisites

- AWS CLI configured (`aws configure`)
- Go 1.21+
- D1 credentials (already set up)

## Build

```sh
GOOS=linux GOARCH=amd64 go build -o bootstrap .
zip lambda.zip bootstrap
```

For ARM (cheaper, faster):

```sh
GOOS=linux GOARCH=arm64 go build -o bootstrap .
zip lambda.zip bootstrap
```

## Create the Lambda function

```sh
aws lambda create-function \
  --function-name krcrackers \
  --runtime provided.al2023 \
  --architectures x86_64 \
  --handler bootstrap \
  --zip-file fileb://lambda.zip \
  --timeout 30 \
  --memory-size 128 \
  --environment Variables="{
    APP_ENV=production,
    CLOUDFLARE_API_TOKEN=your-token,
    CLOUDFLARE_ACCOUNT_ID=your-account-id,
    CLOUDFLARE_DATABASE_ID=735027ae-2327-4561-8e62-538973817b06
  }" \
  --role arn:aws:iam::YOUR_ACCOUNT:role/lambda-krcrackers
```

## Create an API Gateway

```sh
# Create HTTP API
API_ID=$(aws apigatewayv2 create-api \
  --name krcrackers-api \
  --protocol-type HTTP \
  --query 'ApiId' --output text)

# Create integration
INTEGRATION_ID=$(aws apigatewayv2 create-integration \
  --api-id $API_ID \
  --integration-type AWS_PROXY \
  --integration-uri arn:aws:lambda:REGION:ACCOUNT:function:krcrackers \
  --payload-format-version 2.0 \
  --query 'IntegrationId' --output text)

# Create route (catch-all)
aws apigatewayv2 create-route \
  --api-id $API_ID \
  --route-key 'ANY /{proxy+}' \
  --target "integrations/$INTEGRATION_ID"

# Create stage
aws apigatewayv2 create-stage \
  --api-id $API_ID \
  --stage-name '$default' \
  --auto-deploy

# Grant API Gateway permission to invoke Lambda
aws lambda add-permission \
  --function-name krcrackers \
  --statement-id apigateway \
  --action lambda:InvokeFunction \
  --principal apigateway.amazonaws.com \
  --source-arn "arn:aws:execute-api:REGION:ACCOUNT:$API_ID/*"
```

Your API is live at `https://$API_ID.execute-api.REGION.amazonaws.com`.

## Migrations

Migrations run separately from the Lambda function. Execute the binary locally against D1:

```sh
APP_ENV=production \
CLOUDFLARE_API_TOKEN=your-token \
CLOUDFLARE_ACCOUNT_ID=your-account-id \
CLOUDFLARE_DATABASE_ID=735027ae-2327-4561-8e62-538973817b06 \
./bootstrap migrate up
```

Or run from a Lambda invocation:

```sh
aws lambda invoke \
  --function-name krcrackers \
  --payload '{"command":"migrate","args":["up"]}' \
  /tmp/response.json
```

## Update the binary

```sh
GOOS=linux GOARCH=amd64 go build -o bootstrap .
zip lambda.zip bootstrap
aws lambda update-function-code \
  --function-name krcrackers \
  --zip-file fileb://lambda.zip
```

## Low traffic

Lambda free tier covers low-traffic APIs:

- **Free tier:** 1M requests/month, 400K GB-seconds
- **You pay nothing** until you exceed the free tier
- **Cold starts (~100ms)** happen after ~5-15 min of inactivity, acceptable at low traffic
- **Skip provisioned concurrency** — it costs ~$3.50/mo and isn't needed

## SAM template (alternative)

For infrastructure-as-code:

```yaml
# template.yaml
AWSTemplateFormatVersion: '2010-09-09'
Transform: AWS::Serverless-2016-10-31

Globals:
  Function:
    Timeout: 30
    MemorySize: 128
    Runtime: provided.al2023
    Architectures:
      - arm64

Resources:
  ApiFunction:
    Type: AWS::Serverless::Function
    Properties:
      Handler: bootstrap
      CodeUri: .
      Environment:
        Variables:
          APP_ENV: production
          CLOUDFLARE_API_TOKEN: !Ref CloudflareToken
          CLOUDFLARE_ACCOUNT_ID: !Ref CloudflareAccountId
          CLOUDFLARE_DATABASE_ID: !Ref CloudflareDatabaseId
      Events:
        CatchAll:
          Type: HttpApi
          Properties:
            Path: /{proxy+}
            Method: ANY

Outputs:
  ApiUrl:
    Description: "API Gateway endpoint URL"
    Value: !Sub "https://${ServerlessHttpApi}.execute-api.${AWS::Region}.amazonaws.com"
```

Deploy:

```sh
sam build
sam deploy --guided
```

## Makefile targets

```makefile
build-lambda:          ## Build for AWS Lambda
	GOOS=linux GOARCH=amd64 go build -o bootstrap .
	zip lambda.zip bootstrap

deploy-lambda: build-lambda  ## Deploy to AWS Lambda
	aws lambda update-function-code \
		--function-name krcrackers \
		--zip-file fileb://lambda.zip
```
