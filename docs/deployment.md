# Deployment (AWS Lambda)

## Architecture

```
Client → Function URL → Lambda (ap-south-1) → D1 (via HTTPS)
```

Lambda runs your Go binary as a Linux process on ARM64. D1 is accessed over HTTPS using the existing `cloudflare-go/v7` SDK.

## Live endpoint

```
https://65bstxj4hottbqu3cg6rm4bx2m0qsxcr.lambda-url.ap-south-1.on.aws
```

## Performance

| Metric | Value |
|---|---|
| Cold start | ~2s |
| Warm requests | <500ms |
| Region | ap-south-1 (Mumbai) |
| Architecture | arm64 |

Cold starts happen after ~5-15 min of inactivity. Subsequent requests are served from a warm container.

## Prerequisites

- AWS CLI configured (`aws configure`)
- Go 1.21+
- D1 credentials (already set up)

## Build

```sh
GOOS=linux GOARCH=arm64 go build -o bootstrap ./cmd/lambda
zip lambda.zip bootstrap
```

## Create the Lambda function

```sh
aws lambda create-function \
  --function-name krcrackers \
  --region ap-south-1 \
  --runtime provided.al2023 \
  --architectures arm64 \
  --handler bootstrap \
  --zip-file fileb://lambda.zip \
  --timeout 30 \
  --memory-size 128 \
  --environment "Variables={
    APP_ENV=production,
    CLOUDFLARE_API_TOKEN=your-token,
    CLOUDFLARE_ACCOUNT_ID=your-account-id,
    CLOUDFLARE_DATABASE_ID=735027ae-2327-4561-8e62-538973817b06
  }" \
  --role arn:aws:iam::YOUR_ACCOUNT:role/lambda-krcrackers
```

## Create a Function URL

```sh
aws lambda create-function-url-config \
  --function-name krcrackers \
  --region ap-south-1 \
  --auth-type NONE

aws lambda add-permission \
  --function-name krcrackers \
  --region ap-south-1 \
  --statement-id function-url \
  --action lambda:InvokeFunctionUrl \
  --principal '*' \
  --function-url-auth-type NONE
```

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
  --region ap-south-1 \
  --payload '{"command":"migrate","args":["up"]}' \
  /tmp/response.json
```

## Update the binary

```sh
make deploy-lambda
```

Or manually:

```sh
GOOS=linux GOARCH=arm64 go build -o bootstrap ./cmd/lambda
zip lambda.zip bootstrap
aws lambda update-function-code \
  --function-name krcrackers \
  --region ap-south-1 \
  --zip-file fileb://lambda.zip
```

## Low traffic

Lambda free tier covers low-traffic APIs:

- **Free tier:** 1M requests/month, 400K GB-seconds
- **You pay nothing** until you exceed the free tier
- **Cold starts (~2s)** happen after ~5-15 min of inactivity, acceptable at low traffic
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
  FunctionUrl:
    Description: "Lambda Function URL"
    Value: !GetAtt ApiFunctionUrl.FunctionUrl
```

Deploy:

```sh
sam build
sam deploy --guided
```
