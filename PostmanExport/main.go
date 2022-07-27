package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	postman "github.com/rbretecher/go-postman-collection"
)

// used to unmarshal integration test json
type IntegrationTest struct {
	QueryName  string      `json:"queryName"`
	HttpMethod string      `json:"httpMethod"`
	ApiName    string      `json:"apiName"`
	Structure  []Structure `json:"structure"`
}

type Structure struct {
	Env            string                 `json:"env"`
	ResponseCode   int                    `json:"responseCode"`
	ApiParamMap    map[string]interface{} `json:"apiParamMap"`
	Variables      map[string]interface{} `json:"variables"`
	ResponseString map[string]interface{} `json:"responseString"`
}

const (
	defaultHostStaging = "{{hostStaging}}"
	defaultHostProd    = "{{hostProd}}"
)

func main() {
	// file name for postman collection
	fileName := "sampleapp-api.json"

	// collection name and description
	c := postman.CreateCollection("sampleapp-API", "Collection scraped from integration test") // change this

	// integartion test local path
	integrationPath := "/Users/i.wirananta/go/src/sampleapp/ApiIntegrationTest/TestCases" // change this

	// generate success only if true
	isSuccessOnly := false

	// use hardcoded host instead of postman variable
	// if host is empty, will use postman variable e.g. {{hostStaging}}
	// host format : http://sampleapp.service.xxxx.consul
	hostStaging := ""
	hostProd := ""

	if hostStaging == "" {
		hostStaging = defaultHostStaging
	}

	if hostProd == "" {
		hostProd = defaultHostProd
	}

	// create 3 collection for each env
	folderStaging := postman.CreateItemGroup(postman.ItemGroup{
		Name: "Staging",
	})
	folderProd := postman.CreateItemGroup(postman.ItemGroup{
		Name: "Production",
	})
	folderLocal := postman.CreateItemGroup(postman.ItemGroup{
		Name: "Local",
	})

	// same function with string.Split but ignore empty char
	splitFn := func(c rune) bool {
		return c == '/'
	}

	var total int                         // total Integration test processed
	err := filepath.Walk(integrationPath, // will "walk" to every directory in integrationPath
		func(path string, info os.FileInfo, err error) error {
			if path[len(path)-4:] == "json" { // if current file is json, then will proceed
				total++

				jsonFile, err := os.Open(path)
				if err != nil {
					fmt.Println(err)
				}
				defer jsonFile.Close()

				byteValue, _ := ioutil.ReadAll(jsonFile)
				var result IntegrationTest // hold IntegrationTest type
				json.Unmarshal([]byte(byteValue), &result)

				content, _ := ioutil.ReadFile(path)

				// extract value of "variable" straight from json content as string
				variablesStaging, variablesProd := extractValue(string(content), "variables")

				apiName := strings.Replace(result.ApiName, "{host}", "", -1)

				// add to staging collection
				if !isSuccessOnly || result.Structure[0].ResponseCode == 200 {

					// add request for staging
					auth := postman.CreateAuth(postman.NoAuth)
					postmanItemStaging := postman.Items{
						Name: "[" + fmt.Sprintf("%v", result.Structure[0].ResponseCode) + "] " + result.QueryName,
						Request: &postman.Request{
							Auth: auth,
							URL: &postman.URL{
								Raw:      hostStaging + apiName,
								Protocol: "http",
							},
							Header: []*postman.Header{nil},
						},
					}

					switch strings.ToUpper(result.HttpMethod) {
					case "GET":
						postmanItemStaging.Request.Method = postman.Get

						// handle existing route param if exist
						if len(result.Structure[0].ApiParamMap) > 2 {
							for key, data := range result.Structure[0].ApiParamMap {
								if key != "host" && key != "consulHost" {
									switch data.(type) {
									case int:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%d", data), -1)
									case float64:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%.0f", data), -1)
									case string:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%v", data), -1)
									default:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%v", data), -1)
									}
								}
							}
							postmanItemStaging.Request.URL.Raw = hostStaging + apiName
						}

						// create or add route param from variable
						if variablesStaging != "{}" {
							var routeParam string
							if !strings.Contains(apiName, "?") {
								routeParam = "?" // if url has no param, add ?
							} else {
								routeParam = "&" // if url already has param, add &
							}
							for key, data := range result.Structure[0].Variables {
								switch data.(type) {
								case int:
									routeParam += key + `=` + fmt.Sprintf("%d", data) + `&`
								case float64:
									routeParam += key + `=` + fmt.Sprintf("%.0f", data) + `&`
								case string:
									routeParam += key + `=` + fmt.Sprintf("%v", data) + `&`
								default:
									routeParam += key + `=` + fmt.Sprintf("%v", data) + `&`
								}

							}
							if last := len(routeParam) - 1; last >= 0 && routeParam[last] == '&' {
								routeParam = routeParam[:last]
							}
							postmanItemStaging.Request.URL.Raw += strings.Replace(routeParam, "\"", "\\\"", -1)
						}

						// insert query/variable to postman item query
						url, _ := url.Parse(postmanItemStaging.Request.URL.Raw)
						query := []map[string]interface{}{}
						for key, value := range url.Query() {
							query = append(query, map[string]interface{}{
								"key":   key,
								"value": strings.Join(value, ", "),
							})
						}
						postmanItemStaging.Request.URL.Query = query

						if hostStaging != defaultHostStaging {
							postmanItemStaging.Request.URL.Host = strings.Split(url.Host, ".")
							postmanItemStaging.Request.URL.Path = strings.FieldsFunc(url.Path, splitFn)
							postmanItemStaging.Request.URL.Port = url.Port()
						} else {
							postmanItemStaging.Request.URL.Host = []string{defaultHostStaging}
							path := strings.FieldsFunc(apiName, splitFn)
							for i, x := range path {
								if index := strings.Index(x, "?"); index != -1 {
									path[i] = x[:index]
								}
							}
							postmanItemStaging.Request.URL.Path = path
						}
						folderStaging.AddItem(&postmanItemStaging)
						postmanItemLocal := copyPostmanItem(postmanItemStaging, hostStaging, "{{localhost}}", apiName)
						folderLocal.AddItem(&postmanItemLocal)
					case "POST":
						postmanItemStaging.Request.Method = postman.Post
						if len(result.Structure[0].ApiParamMap) > 2 {
							for key, data := range result.Structure[0].ApiParamMap {
								if key != "host" && key != "consulHost" {
									switch data.(type) {
									case int:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%d", data), -1)
									case float64:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%.0f", data), -1)
									case string:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%v", data), -1)
									default:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%v", data), -1)
									}
								}
							}
							postmanItemStaging.Request.URL.Raw = hostStaging + apiName
						}
						if variablesStaging != "{}" {
							postmanItemStaging.Request.Body = &postman.Body{
								Mode: "raw",
								Raw:  variablesStaging,
							}
						}
						if hostStaging != defaultHostStaging { // if not variable, add host info
							url, _ := url.Parse(postmanItemStaging.Request.URL.Raw)
							postmanItemStaging.Request.URL.Host = strings.Split(url.Host, ".")
							postmanItemStaging.Request.URL.Path = strings.FieldsFunc(url.Path, splitFn)
							postmanItemStaging.Request.URL.Port = url.Port()
						} else {
							postmanItemStaging.Request.URL.Host = []string{defaultHostStaging}
							path := strings.FieldsFunc(apiName, splitFn)
							for i, x := range path {
								if index := strings.Index(x, "?"); index != -1 {
									path[i] = x[:index]
								}
							}
							postmanItemStaging.Request.URL.Path = path
						}
						// insert to folder
						folderStaging.AddItem(&postmanItemStaging)
						postmanItemLocal := copyPostmanItem(postmanItemStaging, hostStaging, "{{localhost}}", apiName)
						folderLocal.AddItem(&postmanItemLocal)
					case "PATCH":
						postmanItemStaging.Request.Method = postman.Patch
						if len(result.Structure[0].ApiParamMap) > 2 {
							for key, data := range result.Structure[0].ApiParamMap {
								if key != "host" && key != "consulHost" {
									switch data.(type) {
									case int:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%d", data), -1)
									case float64:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%.0f", data), -1)
									case string:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%v", data), -1)
									default:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%v", data), -1)
									}
								}
							}
							postmanItemStaging.Request.URL.Raw = hostStaging + apiName
						}
						if variablesStaging != "{}" {
							postmanItemStaging.Request.Body = &postman.Body{
								Mode: "raw",
								Raw:  variablesStaging,
							}
						}
						if hostStaging != defaultHostStaging { // if not variable, add host info
							url, _ := url.Parse(postmanItemStaging.Request.URL.Raw)
							postmanItemStaging.Request.URL.Host = strings.Split(url.Host, ".")
							postmanItemStaging.Request.URL.Path = strings.FieldsFunc(url.Path, splitFn)
							postmanItemStaging.Request.URL.Port = url.Port()
						} else {
							postmanItemStaging.Request.URL.Host = []string{defaultHostStaging}
							path := strings.FieldsFunc(apiName, splitFn)
							for i, x := range path {
								if index := strings.Index(x, "?"); index != -1 {
									path[i] = x[:index]
								}
							}
							postmanItemStaging.Request.URL.Path = path
						}
						// insert to folder
						folderStaging.AddItem(&postmanItemStaging)
						postmanItemLocal := copyPostmanItem(postmanItemStaging, hostStaging, "{{localhost}}", apiName)
						folderLocal.AddItem(&postmanItemLocal)
					case "DELETE":
						postmanItemStaging.Request.Method = postman.Delete
						if len(result.Structure[0].ApiParamMap) > 2 {
							for key, data := range result.Structure[0].ApiParamMap {
								if key != "host" && key != "consulHost" {
									switch data.(type) {
									case int:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%d", data), -1)
									case float64:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%.0f", data), -1)
									case string:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%v", data), -1)
									default:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%v", data), -1)
									}
								}
							}
							postmanItemStaging.Request.URL.Raw = hostStaging + apiName
						}
						if variablesStaging != "{}" {
							postmanItemStaging.Request.Body = &postman.Body{
								Mode: "raw",
								Raw:  variablesStaging,
							}
						}
						if hostStaging != defaultHostStaging { // if not variable, add host info
							url, _ := url.Parse(postmanItemStaging.Request.URL.Raw)
							postmanItemStaging.Request.URL.Host = strings.Split(url.Host, ".")
							postmanItemStaging.Request.URL.Path = strings.FieldsFunc(url.Path, splitFn)
							postmanItemStaging.Request.URL.Port = url.Port()
						} else {
							postmanItemStaging.Request.URL.Host = []string{defaultHostStaging}
							path := strings.FieldsFunc(apiName, splitFn)
							for i, x := range path {
								if index := strings.Index(x, "?"); index != -1 {
									path[i] = x[:index]
								}
							}
							postmanItemStaging.Request.URL.Path = path
						}
						// insert to folder
						folderStaging.AddItem(&postmanItemStaging)
						postmanItemLocal := copyPostmanItem(postmanItemStaging, hostStaging, "{{localhost}}", apiName)
						folderLocal.AddItem(&postmanItemLocal)
					case "PUT":
						postmanItemStaging.Request.Method = postman.Put
						if len(result.Structure[0].ApiParamMap) > 2 {
							for key, data := range result.Structure[0].ApiParamMap {
								if key != "host" && key != "consulHost" {
									switch data.(type) {
									case int:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%d", data), -1)
									case float64:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%.0f", data), -1)
									case string:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%v", data), -1)
									default:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%v", data), -1)
									}
								}
							}
							postmanItemStaging.Request.URL.Raw = hostStaging + apiName
						}
						if variablesStaging != "{}" {
							postmanItemStaging.Request.Body = &postman.Body{
								Mode: "raw",
								Raw:  variablesStaging,
							}
						}
						if hostStaging != defaultHostStaging { // if not variable, add host info
							url, _ := url.Parse(postmanItemStaging.Request.URL.Raw)
							postmanItemStaging.Request.URL.Host = strings.Split(url.Host, ".")
							postmanItemStaging.Request.URL.Path = strings.FieldsFunc(url.Path, splitFn)
							postmanItemStaging.Request.URL.Port = url.Port()
						} else {
							postmanItemStaging.Request.URL.Host = []string{defaultHostStaging}
							path := strings.FieldsFunc(apiName, splitFn)
							for i, x := range path {
								if index := strings.Index(x, "?"); index != -1 {
									path[i] = x[:index]
								}
							}
							postmanItemStaging.Request.URL.Path = path
						}
						// insert to folder
						folderStaging.AddItem(&postmanItemStaging)
						postmanItemLocal := copyPostmanItem(postmanItemStaging, hostStaging, "{{localhost}}", apiName)
						folderLocal.AddItem(&postmanItemLocal)
					}
				}

				// add to production collection
				//
				//
				//
				//
				if !isSuccessOnly || result.Structure[1].ResponseCode == 200 {
					// add request for prod
					auth := postman.CreateAuth(postman.NoAuth)
					postmanItemProd := postman.Items{
						Name: "[" + fmt.Sprintf("%v", result.Structure[1].ResponseCode) + "] " + result.QueryName,
						Request: &postman.Request{
							Auth: auth,
							URL: &postman.URL{
								Raw:      hostProd + apiName,
								Protocol: "http",
							},
							Header: []*postman.Header{nil},
						},
					}

					switch strings.ToUpper(result.HttpMethod) {
					case "GET":
						postmanItemProd.Request.Method = postman.Get

						// handle existing route param if exist
						if len(result.Structure[1].ApiParamMap) > 2 {
							for key, data := range result.Structure[1].ApiParamMap {
								if key != "host" && key != "consulHost" {
									switch data.(type) {
									case int:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%d", data), -1)
									case float64:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%.0f", data), -1)
									case string:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%v", data), -1)
									default:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%v", data), -1)
									}
								}
							}
							postmanItemProd.Request.URL.Raw = hostProd + apiName
						}

						// create or add route param from variable
						if variablesProd != "{}" {
							var routeParam string
							if !strings.Contains(apiName, "?") {
								routeParam = "?" // if url has no param, add ?
							} else {
								routeParam = "&" // if url already has param, add &
							}
							for key, data := range result.Structure[1].Variables {
								switch data.(type) {
								case int:
									routeParam += key + `=` + fmt.Sprintf("%d", data) + `&`
								case float64:
									routeParam += key + `=` + fmt.Sprintf("%.0f", data) + `&`
								case string:
									routeParam += key + `=` + fmt.Sprintf("%v", data) + `&`
								default:
									routeParam += key + `=` + fmt.Sprintf("%v", data) + `&`
								}

							}
							if last := len(routeParam) - 1; last >= 0 && routeParam[last] == '&' {
								routeParam = routeParam[:last]
							}
							postmanItemProd.Request.URL.Raw += strings.Replace(routeParam, "\"", "\\\"", -1)
						}

						// insert query/variable to postman item query
						url, _ := url.Parse(postmanItemProd.Request.URL.Raw)
						query := []map[string]interface{}{}
						for key, value := range url.Query() {
							query = append(query, map[string]interface{}{
								"key":   key,
								"value": strings.Join(value, ", "),
							})
						}
						postmanItemProd.Request.URL.Query = query

						if hostProd != defaultHostProd {
							postmanItemProd.Request.URL.Host = strings.Split(url.Host, ".")
							postmanItemProd.Request.URL.Path = strings.FieldsFunc(url.Path, splitFn)
							postmanItemProd.Request.URL.Port = url.Port()
						} else {
							postmanItemProd.Request.URL.Host = []string{defaultHostProd}
							path := strings.FieldsFunc(apiName, splitFn)
							for i, x := range path {
								if index := strings.Index(x, "?"); index != -1 {
									path[i] = x[:index]
								}
							}
							postmanItemProd.Request.URL.Path = path
						}
						folderProd.AddItem(&postmanItemProd)
					case "POST":
						postmanItemProd.Request.Method = postman.Post
						if len(result.Structure[1].ApiParamMap) > 2 {
							for key, data := range result.Structure[1].ApiParamMap {
								if key != "host" && key != "consulHost" {
									switch data.(type) {
									case int:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%d", data), -1)
									case float64:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%.0f", data), -1)
									case string:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%v", data), -1)
									default:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%v", data), -1)
									}
								}
							}
							postmanItemProd.Request.URL.Raw = hostProd + apiName
						}
						if variablesProd != "{}" {
							postmanItemProd.Request.Body = &postman.Body{
								Mode: "raw",
								Raw:  variablesProd,
							}
						}
						if hostProd != defaultHostProd { // if not variable, add host info
							url, _ := url.Parse(postmanItemProd.Request.URL.Raw)
							postmanItemProd.Request.URL.Host = strings.Split(url.Host, ".")
							postmanItemProd.Request.URL.Path = strings.FieldsFunc(url.Path, splitFn)
							postmanItemProd.Request.URL.Port = url.Port()
						} else {
							postmanItemProd.Request.URL.Host = []string{defaultHostProd}
							path := strings.FieldsFunc(apiName, splitFn)
							for i, x := range path {
								if index := strings.Index(x, "?"); index != -1 {
									path[i] = x[:index]
								}
							}
							postmanItemProd.Request.URL.Path = path
						}
						// insert to folder
						folderProd.AddItem(&postmanItemProd)
					case "PATCH":
						postmanItemProd.Request.Method = postman.Patch
						if len(result.Structure[1].ApiParamMap) > 2 {
							for key, data := range result.Structure[1].ApiParamMap {
								if key != "host" && key != "consulHost" {
									switch data.(type) {
									case int:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%d", data), -1)
									case float64:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%.0f", data), -1)
									case string:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%v", data), -1)
									default:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%v", data), -1)
									}
								}
							}
							postmanItemProd.Request.URL.Raw = hostProd + apiName
						}
						if variablesProd != "{}" {
							postmanItemProd.Request.Body = &postman.Body{
								Mode: "raw",
								Raw:  variablesProd,
							}
						}
						if hostProd != defaultHostProd { // if not variable, add host info
							url, _ := url.Parse(postmanItemProd.Request.URL.Raw)
							postmanItemProd.Request.URL.Host = strings.Split(url.Host, ".")
							postmanItemProd.Request.URL.Path = strings.FieldsFunc(url.Path, splitFn)
							postmanItemProd.Request.URL.Port = url.Port()
						} else {
							postmanItemProd.Request.URL.Host = []string{defaultHostProd}
							path := strings.FieldsFunc(apiName, splitFn)
							for i, x := range path {
								if index := strings.Index(x, "?"); index != -1 {
									path[i] = x[:index]
								}
							}
							postmanItemProd.Request.URL.Path = path
						}
						// insert to folder
						folderProd.AddItem(&postmanItemProd)
					case "DELETE":
						postmanItemProd.Request.Method = postman.Delete
						if len(result.Structure[1].ApiParamMap) > 2 {
							for key, data := range result.Structure[1].ApiParamMap {
								if key != "host" && key != "consulHost" {
									switch data.(type) {
									case int:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%d", data), -1)
									case float64:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%.0f", data), -1)
									case string:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%v", data), -1)
									default:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%v", data), -1)
									}
								}
							}
							postmanItemProd.Request.URL.Raw = hostProd + apiName
						}
						if variablesProd != "{}" {
							postmanItemProd.Request.Body = &postman.Body{
								Mode: "raw",
								Raw:  variablesProd,
							}
						}
						if hostProd != defaultHostProd { // if not variable, add host info
							url, _ := url.Parse(postmanItemProd.Request.URL.Raw)
							postmanItemProd.Request.URL.Host = strings.Split(url.Host, ".")
							postmanItemProd.Request.URL.Path = strings.FieldsFunc(url.Path, splitFn)
							postmanItemProd.Request.URL.Port = url.Port()
						} else {
							postmanItemProd.Request.URL.Host = []string{defaultHostProd}
							path := strings.FieldsFunc(apiName, splitFn)
							for i, x := range path {
								if index := strings.Index(x, "?"); index != -1 {
									path[i] = x[:index]
								}
							}
							postmanItemProd.Request.URL.Path = path
						}
						// insert to folder
						folderProd.AddItem(&postmanItemProd)
					case "PUT":
						postmanItemProd.Request.Method = postman.Put
						if len(result.Structure[1].ApiParamMap) > 2 {
							for key, data := range result.Structure[1].ApiParamMap {
								if key != "host" && key != "consulHost" {
									switch data.(type) {
									case int:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%d", data), -1)
									case float64:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%.0f", data), -1)
									case string:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%v", data), -1)
									default:
										apiName = strings.Replace(apiName, "{"+key+"}", fmt.Sprintf("%v", data), -1)
									}
								}
							}
							postmanItemProd.Request.URL.Raw = hostProd + apiName
						}
						if variablesProd != "{}" {
							postmanItemProd.Request.Body = &postman.Body{
								Mode: "raw",
								Raw:  variablesProd,
							}
						}
						if hostProd != defaultHostProd { // if not variable, add host info
							url, _ := url.Parse(postmanItemProd.Request.URL.Raw)
							postmanItemProd.Request.URL.Host = strings.Split(url.Host, ".")
							postmanItemProd.Request.URL.Path = strings.FieldsFunc(url.Path, splitFn)
							postmanItemProd.Request.URL.Port = url.Port()
						} else {
							postmanItemProd.Request.URL.Host = []string{defaultHostProd}
							path := strings.FieldsFunc(apiName, splitFn)
							for i, x := range path {
								if index := strings.Index(x, "?"); index != -1 {
									path[i] = x[:index]
								}
							}
							postmanItemProd.Request.URL.Path = path
						}
						// insert to folder
						folderProd.AddItem(&postmanItemProd)
					}
				}
			}
			return nil
		})
	if err != nil {
		log.Println(err)
	}

	c.AddItem(folderStaging)
	c.AddItem(folderProd)
	c.AddItem(folderLocal)

	file, err := os.Create(fileName)
	if err != nil {
		fmt.Println(err)
	}
	err = c.Write(file, postman.V210)

	if err != nil {
		fmt.Println(err)
		panic(err)
	}
	file.Close()

	// replace nil header with empty header
	//
	// in order to abide by postman collection v210 where
	// header is mandatory, but there are no way to insert empty header
	// as empty field is removed on postman collection write
	// so this "barbaric" solution is implemented
	// where file is read again, and nil header is replaced with empty header
	content, err := ioutil.ReadFile("./" + fileName)
	if err != nil {
		fmt.Println(err)
		panic(err)
	}
	myString := string(content)
	myString = strings.Replace(myString, "\"header\": [\n                            null\n                        ]", "\"header\": []", -1)
	f, _ := os.Create(fileName)
	f.WriteString(myString)

}

// extract value will extract value from json based on key
// will be used to extract variables (body)
// why? its a hassle converting from map[string]interface{} to json
// will return 2 string, one for each env (staging and prod)
func extractValue(body string, key string) (string, string) {
	var varStaging, varProd string
	startStaging := stringIndexNth(body, key, 1)
	startProd := stringIndexNth(body, key, 2)
	var end, openCurlyBracket, closeCurlyBracket int
	if startStaging < 0 {
		varStaging = "{}"
	} else {
		for i := startStaging; i < len(body); i++ {
			if string(body[i]) == "{" {
				openCurlyBracket++
			} else if string(body[i]) == "}" {
				closeCurlyBracket++
				if openCurlyBracket == closeCurlyBracket {
					end = i
					break
				}
			}
		}
		varStaging = body[startStaging+12 : end+1]
	}

	if startProd < 0 {
		varProd = "{}"
	} else {
		for i := startProd; i < len(body); i++ {
			if string(body[i]) == "{" {
				openCurlyBracket++
			} else if string(body[i]) == "}" {
				closeCurlyBracket++
				if openCurlyBracket == closeCurlyBracket {
					end = i
					break
				}
			}
		}
		varProd = body[startProd+12 : end+1]
	}

	return varStaging, varProd
}

// Same as string.Index(), but can find nth index instead of 1st one only
func stringIndexNth(s, key string, n int) int {
	i := 0
	for m := 1; m <= n; m++ {
		x := strings.Index(s[i:], key)
		if x < 0 {
			break
		}
		i += x
		if m == n {
			return i
		}
		i += len(key)
	}
	return -1
}

// copyPostmanItem is used to deepcopy postman items
func copyPostmanItem(copyFrom postman.Items, oldHost, newHost string, apiName string) postman.Items {
	auth := postman.CreateAuth(postman.NoAuth)
	newItem := postman.Items{
		Name: copyFrom.Name,
		Request: &postman.Request{
			Method: copyFrom.Request.Method,
			Auth:   auth,
			URL: &postman.URL{
				Raw:      strings.Replace(copyFrom.Request.URL.Raw, oldHost, newHost, 1),
				Protocol: "http",
			},
			Header: []*postman.Header{nil},
		},
	}
	if copyFrom.Request.Body != nil {
		newItem.Request.Body = &postman.Body{
			Mode: copyFrom.Request.Body.Mode,
			Raw:  copyFrom.Request.Body.Raw,
		}
	}
	if copyFrom.Request.URL.Query != nil {
		newItem.Request.URL.Query = copyFrom.Request.URL.Query
	}

	splitFn := func(c rune) bool {
		return c == '/'
	}

	newItem.Request.URL.Host = []string{newHost}
	newItem.Request.URL.Path = strings.FieldsFunc(apiName, splitFn)

	return newItem
}
