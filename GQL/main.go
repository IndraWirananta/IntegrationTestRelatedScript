package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/360EntSecGroup-Skylar/excelize"
)

// used to unmarshal integration test json
type IntegrationTest struct {
	QueryName string      `json:"queryName"`
	Query     string      `json:"query"`
	Structure []Structure `json:"structure"`
}

// hold value for integration test structure
type Structure struct {
	Env            string                 `json:"env"`
	ResponseCode   int                    `json:"responseCode"`
	ApiParamMap    map[string]interface{} `json:"apiParamMap"`
	Variables      map[string]interface{} `json:"variables"`
	ResponseString map[string]interface{} `json:"responseString"`
}

func main() {
	// gql integration test local path
	integrationPath := "/Users/i.wirananta/go/src/github.com/sampleapp/integrationTest" // change this

	// local path for app queries
	gqlPathQueries := "/Users/i.wirananta/go/src/github.com/sampleapp/queries.go" // change this

	// local path for app mutation
	gqlPathMutation := "/Users/i.wirananta/go/src/github.com/sampleapp/mutations.go" // change this

	// file name for sheet
	documentName := "./ITSWEEP.xlsx"

	// create new excel sheet
	xlsx := excelize.NewFile()
	sheet1Name := "Sheet1"
	xlsx.SetSheetName(xlsx.GetSheetName(1), sheet1Name)

	// create column name
	xlsx.SetCellValue(sheet1Name, "A1", "Endpoint")
	xlsx.SetCellValue(sheet1Name, "B1", "Type")
	xlsx.SetCellValue(sheet1Name, "C1", "Test Case Name")
	xlsx.SetCellValue(sheet1Name, "D1", "File Name")
	xlsx.SetCellValue(sheet1Name, "E1", "Scenario")
	xlsx.SetCellValue(sheet1Name, "F1", "Expected Response")
	xlsx.SetCellValue(sheet1Name, "G1", "Request Param")
	xlsx.SetCellValue(sheet1Name, "H1", "Query")
	xlsx.SetCellValue(sheet1Name, "I1", "Status")
	xlsx.SetCellValue(sheet1Name, "J1", "Notes")
	xlsx.SetCellValue(sheet1Name, "K1", "PIC")

	// add auto filter to column
	err := xlsx.AutoFilter(sheet1Name, "A1", "K1", "")
	if err != nil {
		log.Fatal("ERROR ", err.Error())
	}

	// add data validation for status column
	dvRange := excelize.NewDataValidation(true)
	dvRange.Sqref = "I:I"
	dvRange.SetDropList([]string{"Live", "On Progress", "Not Yet", "Pending", "No TestCase", "Not Checked", "Need Fix", "Wont Do", "Endpoint Need Adjustment"})
	xlsx.AddDataValidation(sheet1Name, dvRange)

	// read queries and mutation file
	gqlQueriesFile, _ := ioutil.ReadFile(gqlPathQueries)
	gqlMutationFile, _ := ioutil.ReadFile(gqlPathMutation)

	// make a map for each queries and mutation
	mapGqlListQueries := regexQueries(string(gqlQueriesFile))
	mapGqlListMutation := regexQueries(string(gqlMutationFile))

	var total int // total integration test

	// used to merge column for the same endpoint
	var prevValue string
	var mergeCellStart int
	var mergeCellEnd int

	err = filepath.Walk(integrationPath, // will "walk" to every directory and subdirectory in integrationPath
		func(path string, info os.FileInfo, err error) error {
			if path[len(path)-4:] == "json" { // if current file is json, then will proceed
				total++

				jsonFile, err := os.Open(path)
				if err != nil {
					fmt.Println(err)
				}
				defer jsonFile.Close()

				byteValue, _ := ioutil.ReadAll(jsonFile)

				var result IntegrationTest
				json.Unmarshal([]byte(byteValue), &result)

				content, _ := ioutil.ReadFile(path)

				// extract value of "variable" straight from json content as string
				// currently only use variable from staging integration test
				variables, _ := extractValue(string(content), `"variables":`)

				endpointName := ""
				// iterate through mapGqlListQueries
				// if found, insert endpointName and queries to sheet
				for key := range mapGqlListQueries {
					if regexCheckEndpoint(result.Query, key) {
						endpointName = key
						mapGqlListQueries[key] = false
						xlsx.SetCellValue(sheet1Name, fmt.Sprintf("B%d", total+1), "Queries")
						break
					}
				}
				// iterate through mapGqlListMutation if endpoint is not found in queries
				// if found, insert endpointName and Mutation to sheet
				if endpointName == "" {
					for key := range mapGqlListMutation {
						if regexCheckEndpoint(result.Query, key) {
							endpointName = key
							mapGqlListMutation[key] = false
							xlsx.SetCellValue(sheet1Name, fmt.Sprintf("B%d", total+1), "Mutation")
							break
						}
					}
				}
				// if not found in mutation or queries, then must be part of chain test case outside of scope
				// will insert endpointName as "Not found in queries/mutation file"
				if endpointName == "" {
					xlsx.SetCellValue(sheet1Name, fmt.Sprintf("B%d", total+1), "-")
					xlsx.SetCellValue(sheet1Name, fmt.Sprintf("J%d", total+1), "Part of chain test case")
					endpointName = "Not found in queries/mutation file"
				}

				// insert data into sheet
				xlsx.SetCellValue(sheet1Name, fmt.Sprintf("A%d", total+1), endpointName)
				xlsx.SetCellValue(sheet1Name, fmt.Sprintf("C%d", total+1), result.QueryName)
				xlsx.SetCellValue(sheet1Name, fmt.Sprintf("D%d", total+1), filepath.Base(path))
				xlsx.SetCellValue(sheet1Name, fmt.Sprintf("F%d", total+1), result.Structure[0].ResponseCode)
				xlsx.SetCellValue(sheet1Name, fmt.Sprintf("G%d", total+1), variables)
				xlsx.SetCellValue(sheet1Name, fmt.Sprintf("H%d", total+1), result.Query)
				xlsx.SetCellValue(sheet1Name, fmt.Sprintf("I%d", total+1), "Live")

				// merge cell if same endpoint
				if prevValue == endpointName && prevValue != "Not found in queries/mutation file" {
					mergeCellEnd += 1
				} else {
					xlsx.MergeCell(sheet1Name, "A"+strconv.Itoa(mergeCellStart), "A"+strconv.Itoa(mergeCellEnd))
					xlsx.MergeCell(sheet1Name, "B"+strconv.Itoa(mergeCellStart), "B"+strconv.Itoa(mergeCellEnd))
					mergeCellStart = total + 1
					mergeCellEnd = mergeCellStart
				}
				prevValue = endpointName

			}
			return nil
		})
	if err != nil {
		log.Println(err)
	}

	// combine both queries and mutation map
	combinedMap := make(map[string]string)
	for k := range mapGqlListMutation {
		if mapGqlListMutation[k] {
			combinedMap[k] = "Mutation"
		}
	}
	for k := range mapGqlListQueries {
		if mapGqlListQueries[k] {
			combinedMap[k] = "Queries"
		}
	}

	// combine both queries and mutation map
	keys := make([]string, 0, len(combinedMap))

	// sort combinedMap
	for k := range combinedMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fmt.Println("Got a total of " + strconv.Itoa(total) + " testcases")

	// insert the rest of combinedMap (endpoint without integration test)
	for _, k := range keys {
		total++
		xlsx.SetCellValue(sheet1Name, fmt.Sprintf("A%d", total+1), k)
		xlsx.SetCellValue(sheet1Name, fmt.Sprintf("B%d", total+1), combinedMap[k])

	}

	fmt.Println("Scanned a total of " + strconv.Itoa(len(mapGqlListQueries)+len(mapGqlListMutation)) + " endpoint")

	// save created sheet
	err = xlsx.SaveAs(documentName)
	if err != nil {
		fmt.Println(err)
	}
}

// regexQueries will scrape for queries/mutation from querier/mutation file
// will return map[string]bool with queries/mutation name as key and true as element
func regexQueries(body string) map[string]bool {
	apiList := make(map[string]bool)
	r := regexp.MustCompile(`([a-zA-Z0-9_]*)\([a-zA-Z0-9_]*\:*[a-zA-Z0-9_ !,:\[\]]*\) *\:`)
	matches := r.FindAllStringSubmatch(body, -1)
	for _, v := range matches {
		apiList[v[1]] = true
	}
	return apiList
}

// regexCheckEndpoint will check if a substring exist in a string
// similar function to strings.index, but handle case such as
// e.g. sampleAppGetProductDetail and sampleAppGetProductDetailFromSomewhere
// strings.index will treat both as the same
func regexCheckEndpoint(body string, key string) bool {
	r := regexp.MustCompile(`[a-zA-Z]*` + key + `[a-zA-Z]*`)
	matches := r.FindAllStringSubmatch(body, -1)
	for _, v := range matches {
		if key == v[0] {
			return true
		}
	}
	return false
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

	nullVariables := strings.Contains(body, `"variables": null`)
	if nullVariables {
		return "null", "null"
	}
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
