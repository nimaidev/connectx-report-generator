package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
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

type TagVO struct {
	CommandId int
	buffer    bytes.Buffer
}

func (t *TagVO) AddByteValue(tag int, value byte) {
	t.buffer.WriteByte(byte(tag))
	t.buffer.WriteByte(value)
}

func (t *TagVO) AddIntValue(tag int, value int) {
	t.buffer.WriteByte(byte(tag))
	binary.Write(&t.buffer, binary.BigEndian, int32(value))
}

func (t *TagVO) AddFloatValue(tag int, value float32) {
	t.buffer.WriteByte(byte(tag))
	binary.Write(&t.buffer, binary.BigEndian, value)
}

func (t *TagVO) AddStringValue(tag int, value string) {
	t.buffer.WriteByte(byte(tag))
	t.buffer.Write([]byte(value))
}

// createRequestMessage returns the byte array for the message
func (t *TagVO) CreateRequestMessage() []byte {
	return t.buffer.Bytes()
}

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
		} else {
			// Generate a value between max and min
			minVal := objectRule.MinValue
			maxVal := objectRule.MaxValue
			randomValue := minVal + rand.Float32()*(maxVal-minVal)
			object.ReportValue = randomValue
			lastValue = object.ReportValue
			log.WithFields(logrus.Fields{
				"ObjectName":  object.ObjectName,
				"objectValue": randomValue,
				"minValue":    minVal,
				"maxValue":    maxVal,
			}).Info("Generated random value between min-max")
		}
		// / Update the object in database
		appConfig.sendReportToController(token, object)
		if err := db.Save(&object); err.Error != nil {
			log.WithError(err.Error).Error("Failed to save object report value")
		}
		//sleep for 10 sec
		time.Sleep(30 * time.Second)
	}
}

func (appConfig AppConfig) sendReportToController(token string, object WiredDeviceObject) error {
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
			data.AddIntValue(3, int(object.ReportValue))
		case FLOAT:
			data.AddFloatValue(3, object.ReportValue)
		case STRING:
			data.AddStringValue(3, fmt.Sprintf("%.2f", object.ReportValue))
		}
	}

	// TAG 4: objectId
	data.AddIntValue(4, int(object.ObjectId))

	// TAG 5: timestamp
	data.AddIntValue(5, int(time.Now().Unix()))

	var output bytes.Buffer
	output.Write(data.CreateRequestMessage())

	//Prepare the payload
	var payload = make(map[int]bytes.Buffer)
	payload[1] = output
	encodedData := base64.StdEncoding.EncodeToString(output.Bytes())
	payloadForm := map[string]interface{}{
		"isRebooted":         "false",
		"uplinkSeqId":        -1,
		"dataFromController": []string{encodedData},
	}

	body, err := json.Marshal(payloadForm)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	//Send data to cloud
	url := fmt.Sprintf("%s/api/gms/sync/v1/from-controller", appConfig.ServerUrl)
	log.Info("URL for heartbeat: ", url)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("error: %v", err)
	}
	// Note: controller variable is not defined in this function scope
	// You may need to pass it as a parameter or define it differently
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("Authorization", "Bearer "+token)

	res, err := GetHttpClient().Do(req)
	if err != nil {
		log.Errorf("HTTP Error : %v", err)
	}

	if res.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("gateway got logged out")

	}

	//read the response for debug
	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		log.Errorf("failed to read response: %v", err)
	}
	log.WithField("response", string(resBody)).Info("Got the response")
	return nil
}
