package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"time"
)

// allow setting multiple destinations
type destArray []string

func (i *destArray) String() string {
	return "string representation"
}

func (i *destArray) Set(value string) error {
	*i = append(*i, value)
	return nil
}

var snapDest destArray

// DoSnapshotRequest - The request body when calling Digital Ocean Droplet Actions API
type DoSnapshotRequest struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

// DoSnapshotResp - the response from the Digital Ocean Droplet Actions API
type DoSnapshotResp struct {
	Action struct {
		ID           int         `json:"id"`
		Status       string      `json:"status"`
		Type         string      `json:"type"`
		StartedAt    time.Time   `json:"started_at"`
		CompletedAt  interface{} `json:"completed_at"`
		ResourceID   int         `json:"resource_id"`
		ResourceType string      `json:"resource_type"`
		Region       struct {
			Name      string   `json:"name"`
			Slug      string   `json:"slug"`
			Sizes     []string `json:"sizes"`
			Features  []string `json:"features"`
			Available bool     `json:"available"`
		} `json:"region"`
		RegionSlug string `json:"region_slug"`
	} `json:"action"`
}

//DoSnapshotStatus - the response for snapshot status
type DoSnapshotStatus struct {
	Action struct {
		ID           int       `json:"id"`
		Status       string    `json:"status"`
		Type         string    `json:"type"`
		StartedAt    time.Time `json:"started_at"`
		CompletedAt  time.Time `json:"completed_at"`
		ResourceID   int       `json:"resource_id"`
		ResourceType string    `json:"resource_type"`
		Region       struct {
			Name      string   `json:"name"`
			Slug      string   `json:"slug"`
			Sizes     []string `json:"sizes"`
			Features  []string `json:"features"`
			Available bool     `json:"available"`
		} `json:"region"`
		RegionSlug string `json:"region_slug"`
	} `json:"action"`
}

// DoSnapshots - the response for listing all snapshots
type DoSnapshots struct {
	Snapshots []struct {
		ID            int           `json:"id,string"`
		Name          string        `json:"name"`
		Regions       []string      `json:"regions"`
		CreatedAt     time.Time     `json:"created_at"`
		ResourceID    interface{}   `json:"resource_id"`
		ResourceType  string        `json:"resource_type"`
		MinDiskSize   int           `json:"min_disk_size"`
		SizeGigabytes float64       `json:"size_gigabytes"`
		Tags          []interface{} `json:"tags"`
	} `json:"snapshots"`
	Links struct {
		Pages struct {
			Last string `json:"last"`
			Next string `json:"next"`
		} `json:"pages"`
	} `json:"links"`
	Meta struct {
		Total int `json:"total"`
	} `json:"meta"`
}

// DoImagesRequest - The request body when calling Digital Ocean Images Actions API
type DoImagesRequest struct {
	Type   string `json:"type"`
	Region string `json:"region"`
}

// DoImagesResp - The response from the Digital Ocean Images Actions API
type DoImagesResp struct {
	Action struct {
		ID           int         `json:"id"`
		Status       string      `json:"status"`
		Type         string      `json:"type"`
		StartedAt    time.Time   `json:"started_at"`
		CompletedAt  interface{} `json:"completed_at"`
		ResourceID   int         `json:"resource_id"`
		ResourceType string      `json:"resource_type"`
		Region       struct {
			Name      string   `json:"name"`
			Slug      string   `json:"slug"`
			Sizes     []string `json:"sizes"`
			Features  []string `json:"features"`
			Available bool     `json:"available"`
		} `json:"region"`
		RegionSlug string `json:"region_slug"`
	} `json:"action"`
}

// SnapshotList - list of matching snapshots for cleanup
type SnapshotList struct {
	Name      string
	ID        int
	CreatedAt time.Time
}

func main() {
	// discover flags
	flag.Var(&snapDest, "d", "Snapshot destination")
	snapKeep := flag.Int("k", -1, "Number of snapshots to keep")
	flag.Parse()

	//if len(snapDest) == 0 {
	//	log.Fatal("You must define at least one snapshot destination '-d'")
	//}

	// discover env variables
	doToken := os.Getenv("DO_TOKEN")

	if len(doToken) == 0 {
		log.Fatal("You must supply a Digital Ocean Token by defining DO_TOKEN environment variable")
	}

	// get droplet ID
	dropletID := getDropletID()

	//snapshot name "autogds-dropletID-YYYYmmddHHmm"
	currentTime := time.Now()
	timestamp := currentTime.Format("200601021504")
	snapshotName := fmt.Sprintf("autogds-%v-%v", dropletID, timestamp)
	snapshotBaseName := fmt.Sprintf("autogds-%v", dropletID)

	if *snapKeep == 0 {
		cleanSnapshots(doToken, *snapKeep, snapshotBaseName)
		os.Exit(0)
	}

	fmt.Println("Taking snapshot", snapshotName, "of droplet ID", dropletID)
	takeSnapshot(doToken, dropletID, snapshotName)
	fmt.Println("Discovering snapshot ID...")
	snapshotID := getSnapshotID(doToken, snapshotName)
	if snapshotID == 0 {
		log.Fatal("ERROR finding snapshot")
	}

	for i := 0; i < len(snapDest); i++ {
		fmt.Println("Transferring", snapshotName, "with ID", snapshotID, "to", snapDest[i])
		transferSnapshot(doToken, snapshotID, snapDest[i])
	}

	if *snapKeep != -1 {
		cleanSnapshots(doToken, *snapKeep, snapshotBaseName)
	}
}

func getDropletID() int {
	// discover droplet ID using local metadata
	uri := "http://169.254.169.254/metadata/v1/id"
	client := &http.Client{}

	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		log.Fatal("Error getting droplet ID:", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal("Error getting droplet ID:", err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal("Error parsing response body:", err)
	}
	dropletIDString := string(body)
	dropletID, _ := strconv.Atoi(string(dropletIDString))

	return dropletID
}

func takeSnapshot(doToken string, dropletID int, snapshotName string) int {
	// take a snapshot of a Droplet
	client := &http.Client{}
	uri := fmt.Sprintf("https://api.digitalocean.com/v2/droplets/%v/actions", dropletID)

	requestBody := DoSnapshotRequest{
		Type: "snapshot",
		Name: snapshotName,
	}

	reqBody, err := json.Marshal(requestBody)
	if err != nil {
		fmt.Println(err)
	}
	u := bytes.NewReader(reqBody)

	req, err := http.NewRequest("POST", uri, u)
	if err != nil {
		log.Fatal(err)
	}

	authHeader := fmt.Sprintf("Bearer %v", doToken)
	req.Header.Add("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}

	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
	} else {
		fmt.Println("Response:", resp)
		fmt.Println("Failed! Status code:", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal("Error parsing Digital Ocean API response body:", err)
	}

	var doSnapshotResp DoSnapshotResp
	jsonErr := json.Unmarshal(body, &doSnapshotResp)
	if jsonErr != nil {
		log.Fatal(jsonErr)
	}

	actionID := doSnapshotResp.Action.ID

	for done := false; !done; {
		fmt.Println("Checking status...")
		status := getActionStatus(doToken, dropletID, actionID, "droplets")

		if status == "" {
			log.Fatal("Error: Unable to retrieve status")
		}

		if status == "errored" {
			log.Fatal("Error creating Snapshot")
		} else if status == "completed" {
			fmt.Println("Snapshot taken successfully!")
			done = true
		} else if status == "in-progress" {
			fmt.Println("In progress...")
			time.Sleep(300 * time.Second)
		}
	}
	return 0
}

func getActionStatus(doToken string, ID int, actionID int, api string) string {
	// query a Digital Ocean action status for "droplets" or "images"
	doAPIuri := fmt.Sprintf("https://api.digitalocean.com/v2/%v/%v/actions/%v", api, ID, actionID)
	client := &http.Client{}

	req, err := http.NewRequest("GET", doAPIuri, nil)
	if err != nil {
		log.Fatal(err)
	}

	authHeader := fmt.Sprintf("Bearer %v", doToken)
	req.Header.Add("Authorization", authHeader)

	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}

	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
	} else {
		fmt.Println("Response:", resp)
		fmt.Println("Failed! Status code:", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal("Error parsing Digital Ocean API response body:", err)
	}

	var doSnapshotStatus DoSnapshotStatus
	jsonErr := json.Unmarshal(body, &doSnapshotStatus)
	if jsonErr != nil {
		log.Fatal(jsonErr)
	}

	status := doSnapshotStatus.Action.Status

	return status
}

func getSnapshotID(doToken string, snapshotName string) int {
	// get the snapshot ID for the created snapshot
	page1 := getSnapshotPage(doToken, 1)

	// see if snapshot is in page1
	for i := 0; i < len(page1.Snapshots); i++ {
		if snapshotName == page1.Snapshots[i].Name {
			return page1.Snapshots[i].ID
		}
	}
	if page1.Links.Pages.Next != "" {
		done := false
		for p := 2; !done; p++ {
			page := getSnapshotPage(doToken, p)
			for i := 0; i < len(page.Snapshots); i++ {
				if snapshotName == page.Snapshots[i].Name {
					return page.Snapshots[i].ID
				}
			}
			if page.Links.Pages.Next == "" {
				done = true
			}
		}
	}
	return 0
}

func getSnapshotPage(doToken string, page int) DoSnapshots {
	// get a page of snapshots
	doAPIuri := fmt.Sprintf("https://api.digitalocean.com/v2/snapshots?page=%v&per_page=50&resource_type=droplet", page)
	client := &http.Client{}

	req, err := http.NewRequest("GET", doAPIuri, nil)
	if err != nil {
		log.Fatal(err)
	}

	authHeader := fmt.Sprintf("Bearer %v", doToken)
	req.Header.Add("Authorization", authHeader)

	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}

	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
	} else {
		fmt.Println("Response:", resp)
		fmt.Println("Failed! Status code:", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal("Error parsing Digital Ocean API response body:", err)
	}

	var doSnapshots DoSnapshots
	jsonErr := json.Unmarshal(body, &doSnapshots)
	if jsonErr != nil {
		log.Fatal(jsonErr)
	}

	return doSnapshots
}

func transferSnapshot(doToken string, snapshotID int, destination string) int {
	// transfer the snapshot to specified destination
	client := &http.Client{}
	uri := fmt.Sprintf("https://api.digitalocean.com/v2/images/%v/actions", snapshotID)

	requestBody := DoImagesRequest{
		Type:   "transfer",
		Region: destination,
	}

	reqBody, err := json.Marshal(requestBody)
	if err != nil {
		fmt.Println(err)
	}
	u := bytes.NewReader(reqBody)

	req, err := http.NewRequest("POST", uri, u)
	if err != nil {
		log.Fatal(err)
	}

	authHeader := fmt.Sprintf("Bearer %v", doToken)
	req.Header.Add("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}

	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
	} else {
		fmt.Println("Response:", resp)
		fmt.Println("Failed! Status code:", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal("Error parsing Digital Ocean API response body:", err)
	}

	var doImagesResp DoImagesResp
	jsonErr := json.Unmarshal(body, &doImagesResp)
	if jsonErr != nil {
		log.Fatal(jsonErr)
	}

	actionID := doImagesResp.Action.ID

	for done := false; !done; {
		fmt.Println("Checking status...")
		status := getActionStatus(doToken, snapshotID, actionID, "images")

		if status == "" {
			log.Fatal("Error: Unable to retrieve status")
		}

		if status == "errored" {
			log.Fatal("Error transferring Snapshot")
		} else if status == "completed" {
			fmt.Println("Snapshot transferred successfully!")
			done = true
		} else if status == "in-progress" {
			fmt.Println("In progress...")
			time.Sleep(300 * time.Second)
		}
	}
	return 0
}

func cleanSnapshots(doToken string, snapKeep int, snapshotBaseName string) {
	// clean up old snapshots
	snapMatch := fmt.Sprintf("^%v", snapshotBaseName)
	r, _ := regexp.Compile(snapMatch)

	page1 := getSnapshotPage(doToken, 1)

	matchedSnapshots := []*SnapshotList{}
	newMatched := new(SnapshotList)

	// see if any matching snapshots are in page1
	for i := 0; i < len(page1.Snapshots); i++ {
		if r.MatchString(page1.Snapshots[i].Name) {
			// add snapshot info to struct
			newMatched = new(SnapshotList)
			newMatched.Name = page1.Snapshots[i].Name
			newMatched.ID = page1.Snapshots[i].ID
			newMatched.CreatedAt = page1.Snapshots[i].CreatedAt
			matchedSnapshots = append(matchedSnapshots, newMatched)
		}
	}
	if page1.Links.Pages.Next != "" {
		done := false
		for p := 2; !done; p++ {
			page := getSnapshotPage(doToken, p)
			for i := 0; i < len(page.Snapshots); i++ {
				if r.MatchString(page.Snapshots[i].Name) {
					// add snapthot info to struct
					newMatched = new(SnapshotList)
					newMatched.Name = page.Snapshots[i].Name
					newMatched.ID = page.Snapshots[i].ID
					newMatched.CreatedAt = page.Snapshots[i].CreatedAt
					matchedSnapshots = append(matchedSnapshots, newMatched)
				}
			}
			if page.Links.Pages.Next == "" {
				done = true
			}
		}
	}

	counter := 0
	for i := 0; i < len(matchedSnapshots); i++ {
		counter++
	}

	sort.Slice(matchedSnapshots, func(i, j int) bool {
		return matchedSnapshots[i].CreatedAt.After(matchedSnapshots[j].CreatedAt)
	})

	if counter > snapKeep {
		for i := snapKeep; i < len(matchedSnapshots); i++ {
			fmt.Println("Deleting snapshot", matchedSnapshots[i].Name)
			deleteSnapshot(doToken, matchedSnapshots[i].ID)
		}
	}
	return
}

func deleteSnapshot(doToken string, snapshotID int) {
	// deletes a snapshot
	doAPIuri := fmt.Sprintf("https://api.digitalocean.com/v2/snapshots/%v", snapshotID)
	client := &http.Client{}

	req, err := http.NewRequest("DELETE", doAPIuri, nil)
	if err != nil {
		log.Fatal(err)
	}

	authHeader := fmt.Sprintf("Bearer %v", doToken)
	req.Header.Add("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}

	if resp.StatusCode == 204 {
		fmt.Println("Successfully requested snapshot deletion for snapshot ID", snapshotID)
	} else {
		fmt.Println("Failed! Status code:", resp.StatusCode)
	}
}
