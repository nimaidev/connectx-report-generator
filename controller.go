package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type Login struct {
	MacAddress string `json:"macAddress"`
	SecretKey  string `json:"secretKey"`
}

func (appConfig AppConfig) StartGateWayOperation(db *gorm.DB) {
	log.Info("Starting gateway operations")
	for {
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
			appConfig.performAuthOperationIfRequired(controller, db)
			appConfig.sendHeartBeatIfRequired(controller, db)
		}
		// sleep for 30s
		time.Sleep(30 * time.Second)
	}
}

func (appConfig AppConfig) sendHeartBeatIfRequired(controller ControllerMaster, db *gorm.DB) {
	timeNow := time.Now()
	shouldSendHeartBeat := false
	if controller.LastHeartBeat.IsZero() {
		shouldSendHeartBeat = true
	} else if timeNow.Sub(controller.LastHeartBeat) > 1*time.Minute {
		shouldSendHeartBeat = true
	}
	if shouldSendHeartBeat {
		controller.LastHeartBeat = timeNow
		appConfig.sendHeartBeat(controller)
		appConfig.saveControllerData(controller, db)
	}
}

func (appConfig AppConfig) sendHeartBeat(controller ControllerMaster) {

}

func (appConfig AppConfig) performAuthOperationIfRequired(controller ControllerMaster, db *gorm.DB) {

	if controller.Password == "" {
		password, err := appConfig.GetSecretKey(controller.MacAddress)
		if err != nil {
			log.Errorf("Error: %v", err)
		}
		controller.Password = password
		err = appConfig.saveControllerData(controller, db)
		if err != nil {
			log.Errorf("Error: %v", err)
		}
	}

	if controller.Token == "" {
		token, err := appConfig.loginGateway(controller.MacAddress, controller.Token)
		if err != nil {
			log.Errorf("Error : %v", err)
		}
		controller.Token = token
		err = appConfig.saveControllerData(controller, db)
		if err != nil {
			log.Errorf("Error: %v", err)
		}
	}

}

func (appConfig AppConfig) saveControllerData(controller ControllerMaster, db *gorm.DB) error {
	if err := db.Save(&controller).Error; err != nil {
		log.WithError(err).WithField("controller_name", controller.ControllerName).Error("Failed to save controller token")
		return err
	} else {
		log.WithField("controller_name", controller.ControllerName).Info("Controller token saved successfully")
		return nil
	}
}

func (appConfig AppConfig) GetSecretKey(macAddress string) (string, error) {
	url := fmt.Sprintf("%s/api/iqnext/controller/v1/nc/getSecretKey/%s", appConfig.ServerUrl, macAddress)
	log.Info("URL for get Key : ", url)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Errorf("Error while creating request %v", err)
	}

	req.Header.Set("Content-Type", "application/json; charset=UTF-8")

	res, err := GetHttpClient().Do(req)
	if err != nil {
		log.Errorf("request failed: %v", err)
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		log.Errorf("HTTP error: %d", res.StatusCode)
	}

	//read the response
	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		log.Errorf("failed to read response: %v", err)
	}

	log.WithField("response", string(resBody)).Info("Got the response")

	// Extract token from nested structure
	var parsed map[string]interface{}
	if err := json.Unmarshal(resBody, &parsed); err != nil {
		return "", fmt.Errorf("failed to parse JSON: %v", err)
	}
	if success, ok := parsed["success"].(map[string]interface{}); ok {
		if data, ok := success["data"].(map[string]interface{}); ok {
			if secretKey, ok := data["secretKey"].(string); ok {
				return secretKey, nil
			}
		}
	}

	return "", fmt.Errorf("secretKey not found in response")
}

func (appConfig AppConfig) loginGateway(macAddress string, password string) (string, error) {

	login := Login{
		MacAddress: macAddress,
		SecretKey:  password,
	}

	body, err := json.Marshal(login)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	// Build the full URL
	url := fmt.Sprintf("%s/api/auth/login/v1/gateway", appConfig.ServerUrl)
	log.Info("", url)

	// Prepare the POST request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")

	res, err := GetHttpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %v", err)
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP error: %d", res.StatusCode)
	}

	//read the response
	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %v", err)
	}

	log.WithField("response", string(resBody)).Info("Got the response")

	// Parse JSON response
	var parsed map[string]interface{}

	if err := json.Unmarshal(resBody, &parsed); err != nil {
		return "", fmt.Errorf("failed to parse JSON: %v", err)
	}

	// Extract token from nested structure
	if success, ok := parsed["success"].(map[string]interface{}); ok {
		if data, ok := success["data"].(map[string]interface{}); ok {
			if token, ok := data["Token"].(string); ok {
				return token, nil
			}
		}
	}

	return "", fmt.Errorf("token not found in response")
}

func GetHttpClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
}
