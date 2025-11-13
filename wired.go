package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type WiredDeviceObject struct {
	Id               uint32    `gorm:"primaryKey" json:"id"`
	OrgId            int8      `json:"orgId"`
	DeviceId         uint32    `json:"deviceId"`
	DeviceName       string    `json:"deviceName"`
	ObjectId         uint32    `json:"objectId"`
	ObjectName       string    `json:"objectName"`
	ControllerId     int16     `json:"controllerId"`
	IqnextObjectType int16     `json:"iqnextObjectType"`
	ReportDataType   int8      `json:"reportDataType"`
	ReportSentAt     time.Time `json:"reportSentAt"`
	ReportType       int8      `json:"reportType"`
	ReportValue      float32   `json:"reportValue"`
}

type WiredObjectRules struct {
	Id           uint16  `gorm:"primaryKey" json:"id"`
	Constant     float32 `json:"constant"`
	MinValue     float32 `json:"minValue"`
	MaxValue     float32 `json:"maxValue"`
	ParamId      int16   `json:"paramId"`
	ParamName    string  `json:"paramName"`
	IsContinuous bool    `json:"isContinuous"`
}

const (
	BYTE = iota + 1
	INTEGER
	FLOAT
	STRING
)

var objectRulesMap = make(map[int16]WiredObjectRules)

func (appConfig AppConfig) LoadObjectRules(db *gorm.DB) {
	var objectRulesList []WiredObjectRules
	result := db.Find(&objectRulesList)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			log.Info("Seems like no rules are present in DB")
		} else {
			log.Errorf("Error; %v", result.Error)
		}
	}
	log.WithFields(logrus.Fields{"rulesCount": len(objectRulesList)}).Info("Found rules details")
	if len(objectRulesList) > 0 {

		for _, rule := range objectRulesList {
			objectRulesMap[rule.ParamId] = rule
		}
		log.Info("Rules Loaded to cache successfully")
	}
}

func (appConfig AppConfig) StartReportGenerationForController(controller ControllerMaster, db *gorm.DB) {
	var wiredDeviceObjectList []WiredDeviceObject
	result := db.Where("controller_id = ?", controller.ControllerId).Find(&wiredDeviceObjectList)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			log.Warn("No controllers found in database")
		} else {
			log.WithError(result.Error).Error("Failed to fetch controllers")
		}
		return
	}
	log.WithFields(logrus.Fields{"count": len(wiredDeviceObjectList), "controller": controller.MacAddress}).Info("Retrieved wired device objects for controller: ")
	for _, object := range wiredDeviceObjectList {
		objectCopy := object // Create a copy to avoid closure issues
		log.Info("Starting to generate report for : " + objectCopy.ObjectName)
		go appConfig.startSendingReportForObject(controller.Token, objectCopy, db)
	}
}

func (appConfig AppConfig) startSendingReportForObject(token string, object WiredDeviceObject, db *gorm.DB) {
	lastValue := object.ReportValue
	for {
		objectRule := objectRulesMap[object.IqnextObjectType]
		if objectRule.IsContinuous {
			// take the last value add the constant and send
			if objectRule.Constant == 0.0 {
				// get a random value between 1
				objectRule.Constant = rand.Float32()
			}
			object.ReportValue = lastValue + objectRule.Constant
			lastValue = object.ReportValue
			log.WithFields(logrus.Fields{"ObjectName": object.ObjectName, "objectValue": lastValue}).Info("Generated value")
		}
		// else {
		// 	// Generate a value between max and min
		// 	minVal := objectRule.MinValue
		// 	maxVal := objectRule.MaxValue
		// 	randomValue := minVal + rand.Float32()*(maxVal-minVal)
		// 	object.ReportValue = randomValue
		// 	lastValue = object.ReportValue
		// 	log.WithFields(logrus.Fields{
		// 		"ObjectName":  object.ObjectName,
		// 		"objectValue": randomValue,
		// 		"minValue":    minVal,
		// 		"maxValue":    maxVal,
		// 	}).Info("Generated random value between min-max")
		// }
		// / Update the object in database
		appConfig.sendReportToController(token, object, 1)
		if err := db.Save(&object); err.Error != nil {
			log.WithError(err.Error).Error("Failed to save object report value")
		}
		//sleep for 10 sec
		time.Sleep(30 * time.Second)
	}
}

func (appConfig AppConfig) sendReportToController(token string, object WiredDeviceObject, reportFor int) error {
	data := &TagVO{CommandId: 1}

	// TAG 1: Bacnet report type
	data.AddByteValue(1, 2)

	// TAG 2: Report value datatype
	data.AddByteValue(2, 4)

	// TAG 3: Report Value
	if object.ReportDataType != 0 {
		switch object.ReportDataType {
		case BYTE:
			data.AddByteValue(3, byte(object.ReportValue))
		case INTEGER:
			data.AddIntValue(3, int32(object.ReportValue))
		case FLOAT:
			data.AddFloatValue(3, object.ReportValue)
		case STRING:
			data.AddStringValue(3, fmt.Sprintf("%.2f", object.ReportValue))
		}
	}

	// TAG 4: objectId
	data.AddIntValue(4, int32(object.ObjectId))

	// TAG 5: timestamp
	data.AddIntValue(5, int32(time.Now().Unix()))

	reportData := data.CreateRequestMessage()

	// Convert bytes to array of integers
	intArray := make([]int, len(reportData))
	for i, b := range reportData {
		intArray[i] = int(b)
	}

	log.Info(intArray)

	// Create map where key = reportFor, value = byte array
	dataArray := make(map[int][]int)
	dataArray[reportFor] = intArray

	payloadForm := map[string]interface{}{
		"isRebooted":         false,
		"uplinkSeqId":        -1,
		"dataFromController": dataArray,
	}

	body, err := json.Marshal(payloadForm)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	log.Info("Sending payload: ", string(body))

	url := fmt.Sprintf("%s/api/gms/sync/v1/from-controller", appConfig.ServerUrl)
	log.Info("URL for heartbeat: ", url)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("error: %v", err)
	}

	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("Authorization", "Bearer "+token)

	res, err := GetHttpClient().Do(req)
	if err != nil {
		return fmt.Errorf("HTTP Error: %v", err)
	}
	defer res.Body.Close()

	log.Infof("Response status: %d", res.StatusCode)

	if res.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("gateway got logged out")
	}

	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		log.Errorf("failed to read response: %v", err)
	}
	log.WithField("response", string(resBody)).Info("Got the response")

	return nil
}
