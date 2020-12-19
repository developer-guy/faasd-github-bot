package function

import (
	"context"
	"fmt"
	"github.com/bradleyfalzon/ghinstallation"
	goGithubV3 "github.com/google/go-github/v32/github"
	githubWebhook "gopkg.in/go-playground/webhooks.v5/github"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
)

var (
	webhookSecret string
	githubClient  *goGithubV3.Client
)

func getAPISecret(secretName string) (secretBytes []byte, err error) {
	// read from the openfaas secrets folder
	secretBytes, err = ioutil.ReadFile("/var/openfaas/secrets/" + secretName)
	if err != nil {
		// read from the original location for backwards compatibility with openfaas <= 0.8.2
		secretBytes, err = ioutil.ReadFile("/run/secrets/" + secretName)
	}

	return secretBytes, err
}

func init() {
	appID, _ := strconv.ParseInt(os.Getenv("APP_ID"), 10, 64)

	privateKeyFile := os.Getenv("PRIVATE_KEY_FILE")
	webhookSecretBytes, err := getAPISecret("webhook-secret")

	if err != nil {
		log.Fatalf("could not read webhook secret: %v", err)
	}

	webhookSecret = string(webhookSecretBytes)

	atr, err := ghinstallation.NewAppsTransportKeyFromFile(http.DefaultTransport, appID, privateKeyFile)
	if err != nil {
		log.Fatalf("error creating GitHub app client: %v", err)
	}

	installation, _, err := goGithubV3.NewClient(&http.Client{Transport: atr}).Apps.FindUserInstallation(context.TODO(), "developer-guy")
	if err != nil {
		log.Fatalf("error finding organization installation: %v", err)
	}

	installationID := installation.GetID()
	itr := ghinstallation.NewFromAppsTransport(atr, installationID)

	log.Printf("successfully initialized GitHub app client, installation-id:%d expected-events:%v\n", installationID, installation.Events)

	githubClient = goGithubV3.NewClient(&http.Client{Transport: itr})
}

func Handle(response http.ResponseWriter, request *http.Request) {
	hook, err := githubWebhook.New(githubWebhook.Options.Secret(webhookSecret))
	if err != nil {
		return
	}

	payload, err := hook.Parse(request, []githubWebhook.Event{githubWebhook.IssuesEvent, githubWebhook.IssueCommentEvent}...)
	if err != nil {
		if err == githubWebhook.ErrEventNotFound {
			log.Printf("received unregistered GitHub event: %v\n", err)
			response.WriteHeader(http.StatusOK)
		} else {
			log.Printf("received malformed GitHub event: %v\n", err)
			response.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	var message string
	switch payloadType := payload.(type) {
	case githubWebhook.IssuesPayload:
		log.Println("received issue event, action:", payloadType)
		issuePayload := payload.(githubWebhook.IssuesPayload)
		message = fmt.Sprintf("Hello, issue opened by: %s", issuePayload.Sender.Login)
		_, _, _ = githubClient.Issues.CreateComment(context.TODO(), issuePayload.Repository.Owner.Login,
			issuePayload.Repository.Name, int(issuePayload.Issue.Number), &goGithubV3.IssueComment{
				Body: &message,
			})
	case githubWebhook.IssueCommentPayload:
		log.Println("received issue comment event, action:", payloadType)
		issueCommentPayload := payload.(githubWebhook.IssueCommentPayload)
		message = fmt.Sprintf("Hello, issue comment added by: %s", issueCommentPayload.Sender.Login)
	default:
		log.Println("missing handler")
	}

	log.Println(message)
}