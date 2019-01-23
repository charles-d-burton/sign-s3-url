# Sign URL
## URL Signer for RSMachiner


### Installation
```bash
$ dep ensure
$ go build -o main
$ zip deployment.zip main
```

### Usage
Place zip file in a Lambda function behind an API gateway.  Send in data that conforms to the User Struct sans CompanyID

### Output
Returns a JSON object containing a signed URL if the request was successful, otherwise returns a 400 with an error message
# sign-s3-url
