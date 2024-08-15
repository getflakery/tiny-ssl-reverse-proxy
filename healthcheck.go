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

type healthCheckError struct {
	Err          error
	DeploymentID string
	Host         string
}

var counter map[string]int = map[string]int{}

// handle method for health check error
func (e healthCheckError) Handle() error {
	fmt.Printf("Deployment: %s, Host: %s UnHealthy\n", e.DeploymentID, e.Host)
	key := e.DeploymentID + e.Host
	if _, ok := counter[key]; ok {
		counter[key]++
	} else {
		counter[key] = 1
	}

	if counter[key] > 8 {
		fmt.Printf("Deployment: %s, Host: %s Marked UnHealthy\n", e.DeploymentID, e.Host)
		return markHostUnhealthy(e.DeploymentID, e.Host)
	}
	return nil
}

func (e healthCheckError) Error() string {
	return fmt.Sprintf("health check error: %v", e.Err)
}

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

				split := strings.Split(servers.URL, ":")
				if len(split) < 2 {
					return fmt.Errorf("url is invalid")
				}
				host := split[0] + ":" + split[1]

				fmt.Printf("Deployment: %s, Host: %s\n", deploymentID, host)
				resp, err := http.Get(host + ":9002/metrics")
				if err != nil {
					return healthCheckError{
						Err:          err,
						DeploymentID: deploymentID,
						Host:         strings.TrimPrefix(split[1], "//"),
					}
				}
				if resp.StatusCode != http.StatusOK {
					return healthCheckError{
						Err:          err,
						DeploymentID: deploymentID,
						Host:         strings.TrimPrefix(split[1], "//"),
					}
				} else {
					fmt.Printf("Deployment: %s, Host: %s Healthy\n", deploymentID, host)
				}
			}
		}
		return nil
	}
}

func healthCheck(ttlCache *TTLCache) error {
	return doEvery(
		5*time.Second,
		getIsHealthy(ttlCache),
		func(err error) {
			switch e := err.(type) {
			case healthCheckError:
				err2 := e.Handle()
				if err2 != nil {
					log.Println(err2)
				}
			default:
				log.Println(err)
			}
		})

}

type UnhealthyHost struct {
	Host string
}

func markHostUnhealthy(deployment string, targetHost string) error {

	// Create the HTTP client
	client := http.Client{}

	body, err := json.Marshal(UnhealthyHost{Host: targetHost})

	if err != nil {
		return errors.Wrap(err, "error marshalling json")
	}

	apihost := os.Getenv("FLAKERY_BASE_URL")
	if apihost == "" {
		fmt.Println("FLAKERY_BASE_URL not set, using default")
		apihost = "http://localhost:3000"
	}

	// Construct the request
	req, err := http.NewRequest("POST", fmt.Sprintf(
		"%s/api/deployments/target/unhealthy/%s",
		apihost, deployment,
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
