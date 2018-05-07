package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/aws/aws-sdk-go/service/s3"
)

//User the representation of a user to retrieve from DynamoDB
type User struct {
	Email       string `json:"email"`
	Sub         string `json:"sub"`
	CompanyID   string `json:"company_id,omitempty"`
	UserName    string `json:"user_name"`
	FileRequest string `json:"file_request"`
	FileSize    int    `json:"file_size"` //Size of the file upload request in bytes
	Payed       bool   `json:"payed,omitempty"`
	ServiceTier int    `json:"service_tier"`
}

//URLSign json object containing signed URL to return back to client
type URLSign struct {
	URL string `json:"url"`
}

//HandleRequest the APIGateway proxy request and return either an error or a signed URL
func HandleRequest(ctx context.Context, event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	sess, err := session.NewSession()
	if err != nil {
		return events.APIGatewayProxyResponse{Body: err.Error()}, nil
	}
	var user User
	err = json.Unmarshal([]byte(event.Body), &user)
	if err != nil {
		return events.APIGatewayProxyResponse{Body: err.Error(), StatusCode: 400}, nil
	}
	valid, err := user.validateUser(sess)
	if !valid || err != nil {
		if err != nil {
			return events.APIGatewayProxyResponse{Body: err.Error(), StatusCode: 400}, nil
		} else {
			return events.APIGatewayProxyResponse{Body: "Invalid User Request", StatusCode: 400}, nil
		}
	}
	url, err := user.signURLForUser(sess)
	if url == "" || err != nil {
		if err != nil {
			return events.APIGatewayProxyResponse{Body: err.Error(), StatusCode: 400}, nil
		} else {
			return events.APIGatewayProxyResponse{Body: "Unable to sign URL", StatusCode: 400}, nil
		}
	}
	var signedURL URLSign
	signedURL.URL = url
	data, err := json.Marshal(&signedURL)
	if err != nil {
		return events.APIGatewayProxyResponse{Body: err.Error(), StatusCode: 400}, nil
	}
	return events.APIGatewayProxyResponse{Body: string(data), StatusCode: 200}, nil
}

//Get the user from dynamo, verify that the "sub" from the current user matches the "sub" stored in dynamo.  set the company_id
func (user *User) validateUser(sess *session.Session) (bool, error) {

	// Create DynamoDB client
	svc := dynamodb.New(sess)
	result, err := svc.GetItem(&dynamodb.GetItemInput{
		TableName: aws.String(os.Getenv("DYNAMO_TABLE")),
		Key: map[string]*dynamodb.AttributeValue{
			"sub": {
				S: aws.String(user.Sub),
			},
		},
	})
	if err != nil {
		return false, err
	}
	if len(result.Item) == 0 { //Response empty meaning the user associated with that sub is not found
		return false, errors.New("User not found")
	}
	var dUser User
	err = dynamodbattribute.UnmarshalMap(result.Item, &dUser)
	if err != nil {
		return false, err
	}
	if dUser.Sub == user.Sub {
		user.CompanyID = dUser.CompanyID
		user.ServiceTier = dUser.ServiceTier
		user.Payed = dUser.Payed
		return true, nil
	}
	return false, errors.New("User invalid")
}

//Check that the user is paid up, and has the correct service tier for the file they're uploading
func (user *User) verifyUserGrants(sess *session.Session) (bool, error) {
	svc := s3.New(sess)
	//Get the objects associated with the user
	objects, _ := svc.ListObjects(&s3.ListObjectsInput{
		Bucket:    aws.String("rsmachiner-user-code"),
		Prefix:    aws.String(user.CompanyID + "/"),
		Delimiter: aws.String("/"),
	})
	var totalSize int64
	for _, object := range objects.Contents {
		size := *object.Size
		totalSize += size
	}
	if user.ServiceTier == 0 {
		if user.FileSize > 10000000 {
			return false, errors.New("File size exceeded tier limit")
		}
		if len(objects.Contents) > 5 {
			return false, errors.New("Number of files in tier exceeded")
		}
		return true, nil
	} else if user.ServiceTier == 1 {

		if totalSize >= 42949672960 || totalSize+int64(user.FileSize) > 42949672960 { // more than 40GB of data
			return false, errors.New("Maximum amount of stored data exceeded")
		}
		return true, nil
	} else if user.ServiceTier == 2 {

	}
	return false, nil
}

//Create the signed url using the company id
func (user *User) signURLForUser(sess *session.Session) (string, error) {
	svc := s3.New(sess)
	req, _ := svc.PutObjectRequest(&s3.PutObjectInput{
		Bucket: aws.String("rsmachiner-user-code"),
		Key:    aws.String(user.CompanyID + "/" + user.FileRequest),
	})
	str, err := req.Presign(5 * time.Minute)
	if err != nil {
		return "", err
	}
	return str, nil
}

//Entrypoint lambda to run code
func main() {
	switch os.Getenv("PLATFORM") {
	case "lambda":
		lambda.Start(HandleRequest)
	default:
		log.Println("no platform defined")
	}
}
