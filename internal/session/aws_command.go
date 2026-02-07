package session

import (
	"fmt"
	"strconv"
)

// BuildSSMPortForwardArgs builds args for:
// aws ssm start-session --document-name AWS-StartPortForwardingSessionToRemoteHost
func BuildSSMPortForwardArgs(targetInstanceID, remoteHost string, remotePort, localPort int, region, profile string) []string {
	args := []string{
		"ssm",
		"start-session",
		"--target", targetInstanceID,
		"--document-name", "AWS-StartPortForwardingSessionToRemoteHost",
		"--parameters", fmt.Sprintf(
			`host=["%s"],portNumber=["%s"],localPortNumber=["%s"]`,
			remoteHost,
			strconv.Itoa(remotePort),
			strconv.Itoa(localPort),
		),
	}

	if region != "" {
		args = append(args, "--region", region)
	}
	if profile != "" {
		args = append(args, "--profile", profile)
	}

	return args
}
