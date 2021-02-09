package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dghubble/go-twitter/twitter"
	"github.com/dghubble/oauth1"
)

// ZipToLatLong defines the json structure of the input data.
// The data comes from: https://public.opendatasoft.com/explore/dataset/us-zip-code-latitude-and-longitude/export/?refine.state=CA
// It is "Flat file JSON".
// Most fields are currently irrelevant, but was simple enough to just
// define the exact structure of the data.
type ZipToLatLong struct {
	DatasetID string `json:"datasetid"`
	RecordID string `json:"recordid"`
	Fields struct {
		City string `json:"city"`
		Zip string `json:"zip"`
		DST int `json:"dst"`
		Geopoint [2]float64 `json:"geopoint"`
		Latitude float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		State string `json:"state"`
		Timezone int `json:"timezone"`
	} `json:"fields"`
	Geometry struct{
		Type string `json:"type"`
		Coordinates [2]float64 `json:"coordinates"`
	} `json:"geometry"`
	RecordTimestamp string `json:"record_timestamp"`
}

const filePath = "./assets/ca-zip-code-latitude-and-longitude.json"

func parseJSONData() ([]*ZipToLatLong, error) {
	var f, err = os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out = new([]*ZipToLatLong)
	var d = json.NewDecoder(f)
	d.DisallowUnknownFields()
	err = d.Decode(out)
	if err != nil {
		return nil, err
	}

	return *out, nil
}

// PostData is the json data included in the POST request to the API.
type PostData struct {
	// From date is a date of the form YYYY-MM-DD.
	FromDate string `json:"fromDate"`
	// Location is the Lat/Long of the search location.
	Location *Location `json:"location"`
	// VaccineData appears to tbe a Basr64 encoded string containing some
	// enum or other constant values collected during the web UI's survey
	// for eligibility.
	VaccineData string `json:"vaccineData"`
}

// Location is the Lat/Long passed in the POST request.
type Location struct{
	Lat float64 `json:"lat"`
	Long float64 `json:"lng"`
}

type Response struct {
	Eligible bool `json:"eligible"`
	VaccineData string `json:"vaccineData"`
	// Don't know what this looks like as we haven't gotten one back yet!
	Locations []*VaccineLocation `json:"locations"`
}

type SiteName string

type VaccineLocation struct{
	DisplayAddress string `json:"displayAddress"`
	DistanceInMeters float64 `json:"distanceInMeters"`
	ExtID string `json:"extId"`
	Location *Location `json:"location"`
	Name SiteName `json:"name"`
	OpenHours []Hours `json:"openHours"`
	Type string `json:"type"`
	VaccineData string `json:"vaccineData"`
}

func (v *VaccineLocation) String() string {
	var hours = make([]string, len(v.OpenHours))
	for i, h := range v.OpenHours {
		hours[i] = h.String()
	}
	return string(v.Name) + "\n" +
		v.DisplayAddress + "\n" +
		strings.Join(hours, "\n")
}

type Hours struct {
	Days []string `json:"days"`
	LocalStart string `json:"localStart"`
	LocalEnd string `json:"localEnd"`
}

func (h *Hours) String() string {
	var out string
	for i, d := range h.Days {
		out += strings.ToUpper(d[:1]) + d[1:]
		if i < len(h.Days) - 1 {
			out += ","
		}
	}
	var start, _ = time.Parse("15:04:05", h.LocalStart)
	var end, _ = time.Parse("15:04:05", h.LocalEnd)
	return out + " - " + start.Format("3:04PM") + "-" + end.Format("3:04PM")
}

const (
	DateFormat = "2006-01-02"
	URL = "https://api.myturn.ca.gov/public/locations/search"
	// VaccineData was generated when I filled out the form as if I was 70+.
	// It base64 decodes to:
	// ["a3qt00000001AdLAAU","a3qt00000001AdMAAU","a3qt00000001AgUAAU","a3qt00000001AgVAAU"]
	VaccineData = "WyJhM3F0MDAwMDAwMDFBZExBQVUiLCJhM3F0MDAwMDAwMDFBZE1BQVUiLCJhM3F0MDAwMDAwMDFBZ1VBQVUiLCJhM3F0MDAwMDAwMDFBZ1ZBQVUiXQ=="
	JSONMimeType = "application/json"

	EnvAPIKey = "API_KEY"
	EnvAPISecret = "API_SECRET"
	EnvAccessToken = "ACCESS_TOKEN"
	EnvAccessSecret = "ACCESS_SECRET"
)

func twitterClient() (*twitter.Client, error) {
	var apiKey, apiSecret, accessToken, accessSecret string
	var ok bool

	apiKey, ok = os.LookupEnv(EnvAPIKey)
	if !ok {
		return nil, errors.New("missing env variable " + EnvAPIKey)
	}

	apiSecret, ok = os.LookupEnv(EnvAPISecret)
	if !ok {
		return nil, errors.New("missing env variable " + EnvAPISecret)
	}

	accessToken, ok = os.LookupEnv(EnvAccessToken)
	if !ok {
		return nil, errors.New("missing env variable " + EnvAccessToken)
	}

	accessSecret, ok = os.LookupEnv(EnvAccessSecret)
	if !ok {
		return nil, errors.New("missing env variable " + EnvAccessSecret)
	}

	var cfg = oauth1.NewConfig(apiKey, apiSecret)
	var token = oauth1.NewToken(accessToken, accessSecret)
	var c = cfg.Client(oauth1.NoContext, token)

	return twitter.NewClient(c), nil
}

func main() {
	var data, err = parseJSONData()
	if err != nil {
		log.Fatal("parsing data: ", err)
	}

	var client *twitter.Client
	client, err = twitterClient()
	if err != nil {
		log.Fatal("failed initializing twitter client: ", err)
	}

	var locs = make(map[SiteName]*VaccineLocation)

	for _, d := range data {
		var pd = &PostData{
			FromDate: time.Now().Format(DateFormat),
			Location: &Location{
				Lat: d.Fields.Latitude,
				Long: d.Fields.Longitude,
			},
			VaccineData: VaccineData,
		}

		var b []byte
		b, err = json.Marshal(pd)
		if err != nil {
			log.Println("error marshalling record: ", pd)
			continue
		}

		var r *http.Response
		r, err = http.Post(URL, JSONMimeType, bytes.NewReader(b))
		if err != nil || r.StatusCode >= http.StatusBadRequest {
			log.Println("error issuing post request: ", err, pd, r.StatusCode)
			continue
		}

		b, err = ioutil.ReadAll(r.Body)
		if err != nil {
			log.Println("reading response body: ", err)
			continue
		}

		var resp = &Response{}
		err = json.Unmarshal(b, resp)
		if err != nil {
			log.Println("unmarshaling response: ", err)
			continue
		}

		for _, loc := range resp.Locations {
			locs[loc.Name] = loc
		}
	}
	for _, v := range locs {
		_, _, err = client.Statuses.Update(formatTweet(v), nil)
		if err != nil {
			log.Println("error tweeting", err, formatTweet(v))
		}
	}
}

func formatTweet(loc *VaccineLocation) string {
	return loc.String() + "\nSign up at: https://myturn.ca.gov/"
}
