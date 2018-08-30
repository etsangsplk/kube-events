package main

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"text/tabwriter"
	"time"

	"github.com/containerum/kube-events/pkg/storage/mongodb"

	"github.com/containerum/kube-events/pkg/model"
	"github.com/containerum/kube-events/pkg/transform"
	"github.com/sirupsen/logrus"
	"gopkg.in/urfave/cli.v2"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

var defaultListOptions = meta_v1.ListOptions{
	Watch: true,
}

var eventTransformer = transform.EventTransformer{
	RuleSelector: func(event watch.Event) string {
		return string(ObservableTypeFromObject(event.Object))
	},
	Rules: map[string]transform.Func{
		string(model.ObservableNamespace):        MakeNamespaceRecord,
		string(model.ObservableDeployment):       MakeDeployRecord,
		string(model.ObservablePod):              MakePodRecord,
		string(model.ObservableService):          MakeServiceRecord,
		string(model.ObservableIngress):          MakeIngressRecord,
		string(model.ObservablePersistentVolume): MakePVRecord,
		string(model.ObservableNode):             MakeNodeRecord,
	},
}

func printFlags(ctx *cli.Context) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.TabIndent|tabwriter.Debug)
	for _, f := range ctx.FlagNames() {
		fmt.Fprintf(w, "Flag: %s\t Value: %s\n", f, ctx.String(f))
	}
	return w.Flush()
}

func pingKube(client *Kube, pingPeriod time.Duration, errChan chan<- error, stopChan <-chan struct{}) {
	ticker := time.NewTicker(pingPeriod)
	httpClient := http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
		Timeout: client.config.Timeout,
	}
	reqUrl, err := url.Parse(client.config.Host)
	if err != nil {
		errChan <- err
		return
	}
	reqUrl.Path = "/healthz"
	req := http.Request{
		Method: http.MethodGet,
		URL:    reqUrl,
	}
	defer ticker.Stop()
	for {
		select {
		case <-stopChan:
			return
		case <-ticker.C:
			logrus.Debug("Ping kube ", req.URL)
			resp, err := httpClient.Do(&req)
			if err != nil {
				errChan <- err
				continue
			}
			body, err := ioutil.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				errChan <- err
				continue
			}
			if resp.StatusCode != http.StatusOK || string(body) != "ok" {
				errChan <- fmt.Errorf("%s", body)
			}
		}
	}
}

func action(ctx *cli.Context) error {
	setupLogs(ctx)

	kubeClient, err := setupKubeClient(ctx)
	if err != nil {
		return err
	}

	watchers, err := kubeClient.WatchSupportedResources()
	if err != nil {
		return err
	}

	mongoStorage, err := setupMongo(ctx)
	if err != nil {
		return err
	}
	defer mongoStorage.Close()

	//Namespaces
	defer watchers.ResourceQuotas.Stop()
	nsBuffer, err := setupBuffer(ctx, mongoStorage, eventTransformer.Output(watchers.ResourceQuotas.ResultChan()))
	if err != nil {
		return err
	}
	defer nsBuffer.Stop()
	go nsBuffer.RunCollection(mongodb.ResourceQuotasCollection)

	//Deployments
	defer watchers.Deployments.Stop()
	deplBuffer, err := setupBuffer(ctx, mongoStorage, eventTransformer.Output(watchers.Deployments.ResultChan()))
	if err != nil {
		return err
	}
	defer deplBuffer.Stop()
	go deplBuffer.RunCollection(mongodb.DeploymentCollection)

	//Pod events
	defer watchers.Events.Stop()
	eventBuffer, err := setupBuffer(ctx, mongoStorage, eventTransformer.Output(watchers.Events.ResultChan()))
	if err != nil {
		return err
	}
	defer eventBuffer.Stop()
	go eventBuffer.RunCollection(mongodb.EventsCollection)

	//Services
	defer watchers.Services.Stop()
	svcBuffer, err := setupBuffer(ctx, mongoStorage, eventTransformer.Output(watchers.Services.ResultChan()))
	if err != nil {
		return err
	}
	defer svcBuffer.Stop()
	go svcBuffer.RunCollection(mongodb.ServiceCollection)

	//Ingresses
	defer watchers.Ingresses.Stop()
	ingrBuffer, err := setupBuffer(ctx, mongoStorage, eventTransformer.Output(watchers.Ingresses.ResultChan()))
	if err != nil {
		return err
	}
	defer ingrBuffer.Stop()
	go ingrBuffer.RunCollection(mongodb.IngressCollection)

	//Volumes
	defer watchers.PVs.Stop()
	pvBuffer, err := setupBuffer(ctx, mongoStorage, eventTransformer.Output(watchers.PVs.ResultChan()))
	if err != nil {
		return err
	}
	defer pvBuffer.Stop()
	go pvBuffer.RunCollection(mongodb.PVCollection)

	pingStopChan := make(chan struct{})
	defer close(pingStopChan)
	pingErrChan := make(chan error)
	go pingKube(kubeClient, 5*time.Second, pingErrChan, pingStopChan)

	sigch := make(chan os.Signal)
	signal.Notify(sigch, os.Kill, os.Interrupt)
	select {
	case <-sigch:
		return nil
	case err := <-pingErrChan:
		logrus.WithError(err).Errorf("Ping kube failed")
		os.Exit(1)
	}

	return nil
}

func main() {
	app := cli.App{
		Name:        "kube-events",
		Description: "Subscribes for kubernetes watches, filters it and records to storage.",
		Flags: []cli.Flag{
			&configFlag,
			&debugFlag,
			&textlogFlag,
			&retentionPeriodFlag,
			&cleanupIntervalFlag,
			&mongoAddressFlag,
			&mongoUserFlag,
			&mongoPasswordFlag,
			&mongoDatabaseFlag,
			&mongoCollectionSizeFlag,
			&mongoCollectionMaxDocsFlag,
			&bufferCapacityFlag,
			&bufferFlushPeriodFlag,
			&bufferMinInsertEventsFlag,
			&connectTimeoutFlag,
		},
		Before: printFlags,
		Action: action,
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Printf("ERROR: %v\n", err)
		os.Exit(1)
	}
}
