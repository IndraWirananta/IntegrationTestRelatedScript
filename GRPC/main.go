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
	QueryName  string      `json:"queryName"`
	HttpMethod string      `json:"httpMethod"`
	ApiName    string      `json:"apiName"`
	Structure  []Structure `json:"structure"`
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
	// grpc integration test local path
	integrationPath := "/Users/i.wirananta/go/src/github.com/sampleapp/grpc_testData" // change this

	// grpc protos local path
	protos := "/Users/i.wirananta/go/src/github.com/sampleapp/protos/sampleapp.proto" // change this

	// repository name, used for regex
	repositoryName := "sampleapp"

	// file name for sheet
	documentName := "./ITSWEEP.xlsx" // change this

	// create new excel sheet
	xlsx := excelize.NewFile()
	sheet1Name := "Sheet1"
	xlsx.SetSheetName(xlsx.GetSheetName(1), sheet1Name)

	// create column name
	xlsx.SetCellValue(sheet1Name, "A1", "Endpoint")
	xlsx.SetCellValue(sheet1Name, "B1", "Test Case Name")
	xlsx.SetCellValue(sheet1Name, "C1", "File Name")
	xlsx.SetCellValue(sheet1Name, "D1", "Scenario")
	xlsx.SetCellValue(sheet1Name, "E1", "Expected Response")
	xlsx.SetCellValue(sheet1Name, "F1", "Request Param")
	xlsx.SetCellValue(sheet1Name, "G1", "Status")
	xlsx.SetCellValue(sheet1Name, "H1", "Notes")
	xlsx.SetCellValue(sheet1Name, "I1", "PIC")

	// add auto filter to column
	err := xlsx.AutoFilter(sheet1Name, "A1", "J1", "")
	if err != nil {
		log.Fatal("ERROR ", err.Error())
	}

	// add data validation for status column
	dvRange := excelize.NewDataValidation(true)
	dvRange.Sqref = "G:G"
	dvRange.SetDropList([]string{"Live", "On Progress", "Not Yet", "Pending", "No TestCase", "Not Checked", "Need Fix", "Wont Do", "Endpoint Need Adjustment"})
	xlsx.AddDataValidation(sheet1Name, dvRange)

	// read protos file
	protosFile, _ := ioutil.ReadFile(protos)

	// make a map for Grpc endpoint list
	mapGrpcList := regex(string(protosFile))

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
				variables, _ := extractValue(string(content), "variables")

				// eg. {host}/function/sampleapp.sampleapp.GetProductDetail/invoke -> GetProductDetail
				apiName := regexGetBareEndpoint(result.ApiName, repositoryName)
				if _, ok := mapGrpcList[apiName]; ok {
					mapGrpcList[apiName] = false // if endpoint has integration test, will tag as false
				}

				// insert data
				xlsx.SetCellValue(sheet1Name, fmt.Sprintf("A%d", total+1), apiName)
				xlsx.SetCellValue(sheet1Name, fmt.Sprintf("B%d", total+1), result.QueryName)
				xlsx.SetCellValue(sheet1Name, fmt.Sprintf("C%d", total+1), filepath.Base(path))
				xlsx.SetCellValue(sheet1Name, fmt.Sprintf("E%d", total+1), result.Structure[0].ResponseCode)
				xlsx.SetCellValue(sheet1Name, fmt.Sprintf("F%d", total+1), variables)
				xlsx.SetCellValue(sheet1Name, fmt.Sprintf("G%d", total+1), "Live")

				// merge cell if same endpoint
				if prevValue == apiName {
					mergeCellEnd += 1
				} else {
					xlsx.MergeCell(sheet1Name, "A"+strconv.Itoa(mergeCellStart), "A"+strconv.Itoa(mergeCellEnd))
					mergeCellStart = total + 1
					mergeCellEnd = mergeCellStart
				}
				prevValue = apiName
			}
			return nil
		})
	if err != nil {
		log.Println(err)
	}

	// used to sort mapGrpcList
	keys := make([]string, 0, len(mapGrpcList))

	for k := range mapGrpcList {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fmt.Println("Got a total of " + strconv.Itoa(total) + " testcases")

	// insert endpoint that doesnt has integration test
	for _, k := range keys {
		if mapGrpcList[k] {
			total++
			xlsx.SetCellValue(sheet1Name, fmt.Sprintf("A%d", total+1), k)
			xlsx.SetCellValue(sheet1Name, fmt.Sprintf("G%d", total+1), "No TestCase")
		}
	}

	fmt.Println("Scanned a total of " + strconv.Itoa(len(mapGrpcList)) + " endpoint")

	// save created sheet
	err = xlsx.SaveAs(documentName)
	if err != nil {
		fmt.Println(err)
	}
}

// regex will scrape for endpoint from route file
func regex(body string) map[string]bool {
	apiList := make(map[string]bool)
	r := regexp.MustCompile(`(rpc )([a-zA-Z0-9]*)`)
	matches := r.FindAllStringSubmatch(body, -1)
	for _, v := range matches {
		apiList[v[2]] = true
	}
	return apiList
}

// regexGetBareEndpoint will use regex to get bare endpoint name from grpc ApiName
// eg. {host}/function/sampleapp.Sampleapp.GetProductInfo/invoke -> GetProductInfo
func regexGetBareEndpoint(body string, repositoryName string) string {
	r := regexp.MustCompile(`({host}/function/` + repositoryName + `.` + strings.Title(repositoryName) + `.)([a-zA-Z0-9]*)(/invoke)`)
	matches := r.FindAllStringSubmatch(body, -1)
	for _, v := range matches {
		return v[2]
	}
	return ""
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
