package main

import (
	"errors"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

func (appConfig AppConfig) StartGateWayOperation(db *gorm.DB) {
	log.Info("Starting gateway operations")

	var controllers []ControllerMaster

	result := db.Find(&controllers)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			log.Warn("No controllers found in database")
		} else {
			log.WithError(result.Error).Error("Failed to fetch controllers")
		}
		return
	}

	log.WithFields(logrus.Fields{
		"count": len(controllers),
	}).Info("Controllers fetched successfully")

	for _, controller := range controllers {
		appConfig.checkGetKey(controller)
	}
}

func (appConfig AppConfig) checkGetKey(controller ControllerMaster) {
	log.WithFields(logrus.Fields{
		"controller_name": controller.ControllerName,
	}).Info("Processing controller")

	if controller.Token == "" {
		log.Info("Token is not there for ", controller.ControllerName, " getting it")
		appConfig.loginGateway(controller.MacAddress)
	}
}

func (appConfig AppConfig) loginGateway(macAddress string) {

}
