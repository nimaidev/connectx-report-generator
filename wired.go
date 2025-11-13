package main

import (
	"errors"
	"math/rand/v2"
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
		go appConfig.startSendingReportForObject(objectCopy, db)
	}
}

func (appConfig AppConfig) startSendingReportForObject(object WiredDeviceObject, db *gorm.DB) {
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

		if err := db.Save(&object); err.Error != nil {
			log.WithError(err.Error).Error("Failed to save object report value")
		}
		//sleep for 10 sec
		time.Sleep(10 * time.Second)
	}
}
