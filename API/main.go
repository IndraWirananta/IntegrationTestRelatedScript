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
	// integration test local path
	integrationPath := "/Users/i.wirananta/go/src/github.com/sampleapp/ApiIntegrationTest/TestCases" // change this

	// app repo local path
	applicationPath := "/Users/i.wirananta/go/src/github.com/sampleapp//http.go" // change this

	// file name for sheet
	documentName := "./ITSWEEP.xlsx" // change this

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
	xlsx.SetCellValue(sheet1Name, "H1", "Status")
	xlsx.SetCellValue(sheet1Name, "I1", "Notes")
	xlsx.SetCellValue(sheet1Name, "J1", "PIC")

	// add auto filter to column
	err := xlsx.AutoFilter(sheet1Name, "A1", "J1", "")
	if err != nil {
		log.Fatal("ERROR ", err.Error())
	}

	// add data validation for status column
	dvRange := excelize.NewDataValidation(true)
	dvRange.Sqref = "H:H"
	dvRange.SetDropList([]string{"Live", "On Progress", "Not Yet", "Pending", "No TestCase", "Not Checked", "Need Fix", "Wont Do", "Endpoint Need Adjustment"})
	xlsx.AddDataValidation(sheet1Name, dvRange)

	// read routes file from repo
	appHttpFile, _ := ioutil.ReadFile(applicationPath)

	// scrape routes file for endpoint list
	mapApiList := regex(string(appHttpFile))

	var total int // total integration test

	// used to merge column for the same endpoint
	// var prevValue string
	// var mergeCellStart int
	// var mergeCellEnd int

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

				var result IntegrationTest // hold IntegrationTest type
				json.Unmarshal([]byte(byteValue), &result)

				content, _ := ioutil.ReadFile(path)

				// extract value of "variable" straight from json content as string
				// currently only use variable from staging integration test
				variables, _ := extractValue(string(content), "variables")

				// eg. {host}/remind/add -> /remind/add
				apiName := strings.Replace(result.ApiName, "{host}", "", -1)
				httpMethod := strings.ToUpper(result.HttpMethod)

				// mark endpoint that has integration test
				if _, ok := mapApiList[apiName+httpMethod]; ok {
					mapApiList[apiName+httpMethod][1] = ""
				}

				// transform route param into variables
				if len(result.Structure[0].ApiParamMap) > 2 {
					routeVariable := "{"
					for key, data := range result.Structure[0].ApiParamMap {
						if key != "host" && key != "consulHost" {
							switch data.(type) {
							case int:
								routeVariable += fmt.Sprintf(`"%v": %d,`, key, data)
							case float64:
								routeVariable += fmt.Sprintf(`"%v": %.0f,`, key, data)
							case string:
								routeVariable += fmt.Sprintf(`"%v": %v,`, key, data)
							default:
								routeVariable += fmt.Sprintf(`"%v": %v,`, key, data)
							}
						}
					}
					// remove trailing ","
					if last := len(routeVariable) - 1; last >= 0 && routeVariable[last] == ',' {
						routeVariable = routeVariable[:last] + "}"
					}
					// remove route param from apiName
					if strings.IndexByte(apiName, '?') != -1 {
						apiName = apiName[:strings.IndexByte(apiName, '?')]
					}
					if len(result.Structure[0].ApiParamMap) > 2 {
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
					}

					fmt.Println(apiName)
					variables = routeVariable
				}

				// insert data to sheet
				xlsx.SetCellValue(sheet1Name, fmt.Sprintf("A%d", total+1), apiName)
				xlsx.SetCellValue(sheet1Name, fmt.Sprintf("B%d", total+1), strings.ToUpper(result.HttpMethod))
				xlsx.SetCellValue(sheet1Name, fmt.Sprintf("C%d", total+1), result.QueryName)
				xlsx.SetCellValue(sheet1Name, fmt.Sprintf("D%d", total+1), filepath.Base(path))
				xlsx.SetCellValue(sheet1Name, fmt.Sprintf("F%d", total+1), result.Structure[0].ResponseCode)
				xlsx.SetCellValue(sheet1Name, fmt.Sprintf("G%d", total+1), variables)
				xlsx.SetCellValue(sheet1Name, fmt.Sprintf("H%d", total+1), "Live")

				// merge cell if same endpoint
				// if prevValue == apiName {
				// 	mergeCellEnd += 1
				// } else {
				// 	xlsx.MergeCell(sheet1Name, "A"+strconv.Itoa(mergeCellStart), "A"+strconv.Itoa(mergeCellEnd))
				// 	xlsx.MergeCell(sheet1Name, "B"+strconv.Itoa(mergeCellStart), "B"+strconv.Itoa(mergeCellEnd))
				// 	mergeCellStart = total + 1
				// 	mergeCellEnd = mergeCellStart
				// }
				// prevValue = apiName
			}
			return nil
		})
	if err != nil {
		log.Println(err)
	}

	// used to sort mapApiList
	keys := make([]string, 0, len(mapApiList))
	for k := range mapApiList {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fmt.Println("Got a total of " + strconv.Itoa(total) + " testcases")

	// insert endpoint that doesnt has integration test
	for _, k := range keys {
		if mapApiList[k][1] != "" {
			total++
			xlsx.SetCellValue(sheet1Name, fmt.Sprintf("A%d", total+1), mapApiList[k][0])
			xlsx.SetCellValue(sheet1Name, fmt.Sprintf("B%d", total+1), mapApiList[k][1])
			if strings.Contains(mapApiList[k][0], "intools") {
				xlsx.SetCellValue(sheet1Name, fmt.Sprintf("H%d", total+1), "Wont Do")
				xlsx.SetCellValue(sheet1Name, fmt.Sprintf("I%d", total+1), "Intools")
			} else {
				xlsx.SetCellValue(sheet1Name, fmt.Sprintf("H%d", total+1), "No TestCase")
			}
		}
	}

	fmt.Println("Scanned a total of " + strconv.Itoa(len(mapApiList)) + " endpoint")

	// save created sheet
	err = xlsx.SaveAs(documentName)
	if err != nil {
		fmt.Println(err)
	}
}

// regex will scrape for endpoint from route file (eg. http.go)
func regex(body string) map[string][]string {
	apiList := make(map[string][]string)
	r := regexp.MustCompile(`r\.(Get|Delete|Patch|Post)\(\"(\/[a-zA-Z0-9\/_]*)`)
	matches := r.FindAllStringSubmatch(body, -1)
	for _, v := range matches {
		temp := strings.ToUpper(v[1])
		apiList[v[2]+temp] = []string{v[2], temp}
	}
	return apiList
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
