# Better CFN Signal
[![Build Status](https://github.com/bdwyertech/better-cfn-signal/workflows/Go/badge.svg?branch=master)](https://github.com/bdwyertech/better-cfn-signal/actions?query=workflow%3AGo+branch%3Amaster)
[![](https://images.microbadger.com/badges/image/bdwyertech/better-cfn-signal.svg)](https://microbadger.com/images/bdwyertech/better-cfn-signal)
[![](https://images.microbadger.com/badges/version/bdwyertech/better-cfn-signal.svg)](https://microbadger.com/images/bdwyertech/better-cfn-signal)

This utility [reports success or failure of a new instance deployment to CloudFormation](https://docs.aws.amazon.com/AWSCloudFormation/latest/APIReference/API_SignalResource.html).  It is intended to be used at the tail end of userdata.  The [Amazon cfn-signal](https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/cfn-signal.html) requires a few arguments, including CF Stack ID, Stack Resource Name, and the AWS Region.  This requires effort and is not "batteries included", in the event a user just fires up a new CF stack and does not update UserData.

This utility derives this information from the instance's tags.  The idea here is you give your EC2 an Instance Role capable of reading its own tags, we read them and determine the ResourceID and Cloudformation Stack, rather than having to pass this information via UserData.

#### Required Tags
Both of these tags are automatically applied to the EC2 instance upon creation via CloudFormation.
* `aws:cloudformation:logical-id`
* `aws:cloudformation:stack-name`

#### Required IAM Permissions
The EC2 must also be able to read its own tags, as well as use the CloudFormation SignalResource API.
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Sid": "BetterCfnSignal",
      "Action": [
      	"cloudformation:SignalResource",
        "ec2:DescribeTags"
      ],
      "Resource": "*"
    }
  ]
}
```

### Sample Userdata

#### Linux
```bash
#!/bin/bash -e

echo 'Do some stuff...'

# Signal Success
better-cfn-signal
```

#### Windows
```powershell
<powershell>
$ErrorActionPreference = "Stop"

Write-Host 'Do some stuff...'

# Signal Success
better-cfn-signal
</powershell>
```


### Healthcheck Support
Optionally, Better CFN Signal can wait for a URL to return a 200 prior to sending a healthy response back to CloudFormation.

This was intended for use with the [Go-Healthz healthcheck daemon.](https://github.com/bdwyertech/go-healthz)

```bash
#!/bin/bash -e

echo 'Do some stuff...'

# Signal Success after waiting up to 10 minutes for the Healthcheck URL to return 200
better-cfn-signal -healthcheck-url http://127.0.0.1:8080 -healthcheck-timeout 10m
```
