//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/gomodule/redigo/redis"
	"google.golang.org/api/iterator"
)

var (
	redisPool *redis.Pool
	client    *firestore.Client
)

func incrementHandler(w http.ResponseWriter, r *http.Request) {
	conn := redisPool.Get()
	defer conn.Close()

	ctx := context.Background()

	userWithKey := r.Header.Get("X-Goog-Authenticated-User-Email")
	user := strings.Split(userWithKey, ":")
	now := time.Now()
	paddedSecs := now.Format("05")
	counter, err := redis.Int(conn.Do("INCR", "visits:"+user[1]+":"+paddedSecs))
	conn.Do("EXPIRE", "visits:"+user[1]+":"+paddedSecs, 30)
	if err != nil {
		http.Error(w, "Error incrementing visitor counter", http.StatusInternalServerError)
		return
	}
	if counter > 5 {
		fmt.Fprintf(w, "Rate limit exceeded: %d\n", counter)
		log.Printf("client: rate limit exceeded: %d", counter)
	} else {
		fmt.Fprintf(w, "Welcome %s,\n", user[1])
		fmt.Fprintf(w, "Visitor number: %d\n", counter)
		fmt.Fprintf(w, "Visitor key: %s\n", "visits:"+user[1]+":"+paddedSecs)
		q1 := firestore.PropertyFilter{
			Path:     "data_class",
			Operator: "==",
			Value:    "pii",
		}
		q2 := firestore.PropertyFilter{
			Path:     "data_class",
			Operator: "==",
			Value:    "phi",
		}
		orFilter := firestore.OrFilter{
			Filters: []firestore.EntityFilter{q1, q2},
		}
		orQuery := client.Collection("gcp").WhereEntity(orFilter)
		iter := orQuery.Documents(ctx)
		defer iter.Stop()
		for {
			doc, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				// TODO: Handle error.
			}
			json, _ := json.MarshalIndent(doc.Data(), "", "  ")
			fmt.Fprintf(w, "Firestore Doc size: %d bytes\n", len(string(json)))
			fmt.Fprintf(w, "Firestore Doc: %s \n", string(json))
		}
	}
}

func nullHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "url not found", http.StatusNotFound)
}

func main() {
	var (
		Version      string = "v1.0.0"
		OperatingEnv string = "development"
	)
	log.Printf("Running Version %s in %s", Version, OperatingEnv)

	ctx := context.Background()

	mdEndPoint := "computeMetadata/v1/project/project-id"
	reqMdURL := fmt.Sprintf("http://metadata.google.internal/%s", mdEndPoint)
	req, err := http.NewRequest(http.MethodGet, reqMdURL, nil)
	if err != nil {
		log.Printf("client: could not create request: %s", err)
	}

	req.Header.Set("Metadata-Flavor", "Google")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("client: error making http request: %s", err)
	}

	projectId, err := io.ReadAll(res.Body)
	if err != nil {
		log.Printf("client: could not read response body: %s", err)
	}
	log.Printf("Operating in project id: %s", projectId)

	client, err = firestore.NewClientWithDatabase(ctx, string(projectId), "service-accounts")
	if err != nil {
		log.Fatalf("firestore new error:%s\n", err)
	}
	defer client.Close()

	redisAuth := os.Getenv("REDISAUTH")
	redisHost := os.Getenv("REDISHOST")
	redisPort := os.Getenv("REDISPORT")
	redisAddr := fmt.Sprintf("redis://%s@%s:%s", redisAuth, redisHost, redisPort)

	const maxConnections = 10
	redisPool = &redis.Pool{
		MaxIdle: maxConnections,
		Dial: func() (redis.Conn, error) {
			return redis.DialURL(redisAddr)
		},
	}

	http.HandleFunc("/", incrementHandler)
	http.HandleFunc("/favicon.ico", nullHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Listening on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}
