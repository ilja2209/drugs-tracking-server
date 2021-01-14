package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

type drug struct {
	Name    string `json:"name"`
	Time    string `json:"time"`
	Comment string `json:"comment"`
	Status  bool   `json:"status"`
}

type person struct {
	PersonName string `json:"personName"`
	Drugs      []drug `json:"drugs"`
}

var defaultFilePath = "C:\\temp\\settings.json"
var filePathEnvVariable = "DT_SETTINGS_FILE_PATH"

func getEnvOrDefault(key string, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}

	return defaultValue
}

func getDrugsSettingsHandler(writer http.ResponseWriter, request *http.Request) {
	people, err := loadDrugs()
	if err != nil {
		http.Error(writer, err.Error(), 500)
		return
	}
	marshaledPeople, err := json.Marshal(people)
	fmt.Println(string(marshaledPeople))

	writer.Header().Set("Content-Type", "application/json")
	writer.Write(marshaledPeople)
}

func getDrugsHandler(writer http.ResponseWriter, request *http.Request) {
	err := checkAndResetPendingStatus()
	if err != nil {
		http.Error(writer, err.Error(), 500)
		return
	}
	people, err := getPendingDrugs()
	if err != nil {
		http.Error(writer, err.Error(), 500)
		return
	}
	marshaledPeople, err := json.Marshal(people)
	writer.Header().Set("Content-Type", "application/json")
	writer.Write(marshaledPeople)
}

func checkAndResetPendingStatus() error {
	people, err := loadDrugs()
	if err != nil {
		return err
	}
	if isResetPendingStatus(people) {
		return resetPendingStatus(people)
	}
	return nil
}

func isResetPendingStatus(people []person) bool {
	times := make([]time.Time, 0)
	for _, p := range people {
		for _, d := range p.Drugs {
			times = append(times, convertStr2Time(d.Time))
		}
	}
	sort.SliceStable(times, func(i, j int) bool { return times[i].Before(times[j]) })
	return len(times) > 0 && times[0].After(time.Now())
}

func resetPendingStatus(people []person) error {
	var updatedPeople []person
	for _, p := range people {
		for _, d := range p.Drugs {
			var err error
			if updatedPeople == nil {
				updatedPeople, err = changeStatus(people, p.PersonName, d.Name, false)
				if err != nil {
					return err
				}
			} else {
				updatedPeople, err = changeStatus(updatedPeople, p.PersonName, d.Name, false)
				if err != nil {
					return err
				}
			}
		}
	}

	if updatedPeople != nil {
		err := saveDrugs(updatedPeople)
		return err
	}

	return nil
}

func setDrugsSettingsHandler(writer http.ResponseWriter, request *http.Request) {
	buff, err := ioutil.ReadAll(request.Body)
	defer request.Body.Close()
	if err != nil {
		http.Error(writer, err.Error(), 500)
		return
	}
	persons := make([]person, 0)
	err = json.Unmarshal(buff, &persons)
	if err != nil {
		http.Error(writer, err.Error(), 500)
		return
	}
	err = saveDrugs(persons)
	if err != nil {
		http.Error(writer, err.Error(), 500)
		return
	}
}

func changeDrugStatusHandler(writer http.ResponseWriter, request *http.Request) {
	vars := mux.Vars(request)
	personName := vars["personName"]
	drugName := vars["drugName"]
	people, err := loadDrugs()
	if err != nil {
		http.Error(writer, err.Error(), 500)
		return
	}
	updatedPople, err := changeStatus(people, personName, drugName, true)
	if err != nil {
		http.Error(writer, err.Error(), 500)
		return
	}
	err = saveDrugs(updatedPople)
	if err != nil {
		http.Error(writer, err.Error(), 500)
		return
	}
}

func loadDrugs() ([]person, error) {
	buff, err := ioutil.ReadFile(getEnvOrDefault(filePathEnvVariable, defaultFilePath))
	if err != nil {
		return nil, err
	}
	people := make([]person, 0)
	err = json.Unmarshal(buff, &people)
	return people, err
}

func saveDrugs(people []person) error {
	marshaledPeople, err := json.Marshal(people)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(getEnvOrDefault(filePathEnvVariable, defaultFilePath), marshaledPeople, 0777)
	return err
}

func changeStatus(people []person, personName string, drugName string, status bool) ([]person, error) {
	peopleMap := make(map[string][]drug)
	for _, p := range people {
		peopleMap[p.PersonName] = p.Drugs
	}
	peopleMap[personName] = changeStatusForDrug(peopleMap[personName], drugName, status)
	updatedPeoplesStruct := make([]person, 0)
	for key, value := range peopleMap {
		p := person{
			PersonName: key,
			Drugs:      value,
		}
		updatedPeoplesStruct = append(updatedPeoplesStruct, p)
	}
	return updatedPeoplesStruct, nil
}

func changeStatusForDrug(drugs []drug, drugName string, status bool) []drug {
	updatedDrugs := make([]drug, len(drugs))
	copy(updatedDrugs, drugs)
	for i, d := range drugs {
		if d.Name == drugName {
			updatedDrugs[i].Status = status
		}
	}

	return updatedDrugs
}

func filterDrugs(drugs []drug) []drug {
	filteredDrugs := make([]drug, 0)
	for _, d := range drugs {
		if isBeforeCurrentTime(d.Time) && d.Status == false {
			filteredDrugs = append(filteredDrugs, d)
		}
	}
	return filteredDrugs
}

func isBeforeCurrentTime(scheduleTimeStr string) bool {
	currentTime := time.Now()
	scheduleTime := convertStr2Time(scheduleTimeStr)
	return scheduleTime.Before(currentTime)
}

func convertStr2Time(timeStr string) time.Time {
	currentTime := time.Now()
	s := strings.Split(timeStr, ":")
	hours, err := strconv.Atoi(s[0])
	if err != nil {
		panic(err)
	}
	minutes, err := strconv.Atoi(s[1])
	if err != nil {
		panic(err)
	}
	loc, _ := time.LoadLocation("Europe/Moscow")
	return time.Date(currentTime.Year(), currentTime.Month(), currentTime.Day(), hours, minutes, 00, 0, loc)
}

func getPendingDrugs() ([]person, error) {
	people, err := loadDrugs()
	if err != nil {
		return nil, err
	}
	pendingDrugs := make([]person, 0)
	for _, p := range people {
		drugs := filterDrugs(p.Drugs)
		if len(drugs) > 0 {
			newPerson := person{
				PersonName: p.PersonName,
				Drugs:      drugs,
			}
			pendingDrugs = append(pendingDrugs, newPerson)
		}
	}
	return pendingDrugs, nil
}

func main() {

	router := mux.NewRouter().StrictSlash(true)

	router.HandleFunc("/api/v1/drugs/settings", getDrugsSettingsHandler).Methods("GET")
	router.HandleFunc("/api/v1/drugs", getDrugsHandler).Methods("GET")
	router.HandleFunc("/api/v1/drugs/settings", setDrugsSettingsHandler).Methods("POST")
	router.HandleFunc("/api/v1/drugs/{personName}/{drugName}", changeDrugStatusHandler).Methods("PUT")

	router.HandleFunc("/api/v1", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "OK")
	}).Methods("GET", "POST")

	srv := &http.Server{
		Handler:      router,
		Addr:         ":8080",
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	fmt.Printf("Server is running")
	log.Fatal(srv.ListenAndServe())
}
