package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/pkg/errors"
)

func doEvery(
	d time.Duration,
	f func(time.Time) error,
	errHandler func(error),
) error {
	for x := range time.Tick(d) {
		if err := f(x); err != nil {
			errHandler(err)
		}
	}
	return nil
}

func getIsHealthy(
	ttlCache *TTLCache,
	placeholder func(string, string) error,
) func(t time.Time) error {
	return func(t time.Time) error {
		r, err := ttlCache.Get()
		if err != nil {
			log.Println(err)
		}
		if err != nil {
			return errors.Wrap(err, "error getting cache")
		}
		// fmt.Fprintf(w, "Cache: %s\n", c)
		config, err := parseConfig(r)
		if err != nil {
			return errors.Wrap(err, "error parsing config")
		}
		for deploymentID, services := range config.Http.Services {
			for _, servers := range services.Servers {
				fmt.Printf("Deployment: %s, url: %s\n", deploymentID, servers.URL)
				host := strings.Split(servers.URL, ":")[0]
				fmt.Printf("Deployment: %s, Host: %s\n", deploymentID, host)
				resp, err := http.Get(host + ":9002/metrics")
				if err != nil {
					return errors.Wrap(err, "error getting metrics")
				}
				if resp.StatusCode != http.StatusOK {
					fmt.Printf("Deployment: %s, Host: %s UnHealthy\n", deploymentID, host)

					err := placeholder(deploymentID, host)
					if err != nil {
						return errors.Wrap(err, "error calling placeholder")
					}
				} else {
					fmt.Printf("Deployment: %s, Host: %s Healthy\n", deploymentID, host)
				}
			}
		}
		return nil
	}
}

func healthCheck(ttlCache *TTLCache, placeholder func(string, string) error) error {
	return doEvery(
		5*time.Second,
		getIsHealthy(ttlCache, placeholder),
		func(err error) {
			log.Printf("health check failed: %v", err)
		})

}

type UnhealthyHost struct {
	Host string
}

func placeholder(deployment string, targetHost string) error {

	// Create the HTTP client
	client := http.Client{}

	body, err := json.Marshal(UnhealthyHost{Host: targetHost})

	if err != nil {
		return errors.Wrap(err, "error marshalling json")
	}

	apihost := os.Getenv("FLAKERY_API_HOST")
	if apihost == "" {
		return fmt.Errorf("FLAKERY_API_HOST not set")
	}

	// Construct the request
	req, err := http.NewRequest("POST", fmt.Sprintf(
		apihost, deployment, targetHost,
	), bytes.NewBuffer(body))
	if err != nil {
		return errors.Wrap(err, "error creating request")
	}

	// get flakery api key from env
	apitoken := os.Getenv("FLAKERY_API_KEY")
	if apitoken == "" {
		return fmt.Errorf("FLAKERY_API_KEY not set")
	}

	// Add headers or any other necessary request modifications
	req.Header.Add("Authorization", "Bearer "+apitoken)

	_, err = client.Do(req)
	return errors.Wrap(err, "error making request")
}

func printholder(deployment string, targetHost string) error {
	fmt.Printf("Deployment: %s, Host: %s\n", deployment, targetHost)
	return nil
}
