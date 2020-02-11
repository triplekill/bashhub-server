/*
 *
 * Copyright © 2020 nicksherron <nsherron90@gmail.com>
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"sync"

	"github.com/nicksherron/bashhub-server/internal"
	"github.com/spf13/cobra"
)

type cList struct {
	UUID    string `json:"uuid"`
	Command string `json:"command"`
	Created int64  `json:"created"`
}

type commandRecord struct {
	Command    string `json:"command"`
	Path       string `json:"path"`
	Created    int64  `json:"created"`
	UUID       string `json:"uuid"`
	ExitStatus int    `json:"exitStatus"`
	Username   string `json:"username"`
	SystemName string `json:"systemName"`
	SessionID  string `json:"sessionId"`
}

type commandsList []cList

var (
	srcUser    string
	dstUser    string
	srcURL     string
	dstURL     string
	srcPass    string
	dstPass    string
	srcToken   string
	dstToken   string
	wg         sync.WaitGroup
	cmdList    commandsList
	migrateCmd = &cobra.Command{
		Use:   "migrate",
		Short: "migrate bashhub history ",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Flags().Parse(args)
			srcToken = getToken(srcURL, srcUser, srcPass)
			dstToken = getToken(dstURL, dstUser, dstPass)
			cmdList = getCommandList()
			counter := 0
			for _, v := range cmdList {
				wg.Add(1)
				counter++
				go func(c cList) {
					defer wg.Done()
					commandLookup(c.UUID)
				}(v)
				if counter > 20 {
					wg.Wait()
					counter = 0
				}
			}
			wg.Wait()
		},
	}
)

func init() {
	rootCmd.AddCommand(migrateCmd)
	migrateCmd.PersistentFlags().StringVar(&srcURL, "src-url", "https://bashhub.com", "source url")
	migrateCmd.PersistentFlags().StringVar(&srcUser, "src-user", "", "source username")
	migrateCmd.PersistentFlags().StringVar(&srcPass, "src-pass", "", "source password")
	migrateCmd.PersistentFlags().StringVar(&dstURL, "dst-url", "http://localhost:8080", "destination url")
	migrateCmd.PersistentFlags().StringVar(&dstUser, "dst-user", "", "destination username")
	migrateCmd.PersistentFlags().StringVar(&dstPass, "dst-pass", "", "destination password")

}

func getToken(site string, user string, pass string) string {
	// function used by bashhub to identify system
	cmd := exec.Command("python", "-c", "import uuid; print(str(uuid.getnode()))")
	m, err := cmd.Output()
	if err != nil {
		log.Fatal(err)
	}
	mac := strings.ReplaceAll(string(m), "\n", ``)
	auth := map[string]interface{}{
		"username": user,
		"password": pass,
		"mac":      mac,
	}

	payloadBytes, err := json.Marshal(auth)
	if err != nil {
		log.Fatal(err)
	}

	body := bytes.NewReader(payloadBytes)

	u := fmt.Sprintf("%v/api/v1/login", site)
	req, err := http.NewRequest("POST", u, body)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Fatal("login failed for ", site)
	}
	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	j := make(map[string]interface{})

	json.Unmarshal(buf, &j)
	if len(j) == 0 {
		log.Fatal("login failed for ", site)

	}
	return fmt.Sprintf("Bearer %v", j["accessToken"])
}

func getCommandList() commandsList {
	u := strings.TrimSpace(srcURL) + "/api/v1/command/search?unique=true&limit=1000000"
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		log.Fatal(err)
	}

	req.Header.Add("Authorization", srcToken)

	client := &http.Client{}
	resp, err := client.Do(req)

	if err != nil {
		log.Println("Error on response.\n[ERRO] -", err)
	}

	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Fatalf("failed to get command list from %v, go status code %v", srcURL, resp.StatusCode)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	var result commandsList
	json.Unmarshal(body, &result)

	return result
}

func commandLookup(uuid string) {
	u := strings.TrimSpace(srcURL) + "/api/v1/command/" + strings.TrimSpace(uuid)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		log.Fatal(err)
	}

	req.Header.Add("Authorization", srcToken)

	client := &http.Client{}
	resp, err := client.Do(req)

	if err != nil {
		log.Println("Error on response.\n[ERRO] -", err)
	}

	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Fatalf("failed command lookup from %v, go status code %v", srcURL, resp.StatusCode)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	srcSend(body)
	var j internal.Query
	err = json.Unmarshal(body, &j)
	if err != nil {
		log.Fatal(err)
	}

	j.Username = dstUser
	b, err := json.Marshal(j)
	if err != nil {
		log.Fatal(err)
	}
	srcSend(b)
}

func srcSend(data []byte) {
	body := bytes.NewReader(data)

	u := dstURL + "/api/v1/import"
	req, err := http.NewRequest("POST", u, body)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Add("Authorization", dstToken)
	client := &http.Client{}
	resp, err := client.Do(req)

	if err != nil {
		log.Println("Error on response.\n[ERRO] -", err)
	}

	defer resp.Body.Close()
}