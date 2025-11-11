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
		token := appConfig.checkGetKey(controller)
		if token != "" {
			controller.Token = token
			// Save the updated controller with token
			if err := db.Save(&controller).Error; err != nil {
				log.WithError(err).WithField("controller_name", controller.ControllerName).Error("Failed to save controller token")
			} else {
				log.WithField("controller_name", controller.ControllerName).Info("Controller token saved successfully")
			}
		}
	}
}

func (appConfig AppConfig) checkGetKey(controller ControllerMaster) string {
	log.WithFields(logrus.Fields{
		"controller_name": controller.ControllerName,
	}).Info("Processing controller")
	flag := false
	if controller.Password == "" {
		key, err := appConfig.GetSecretKey(controller.MacAddress)
		if err != nil {
			log.Errorf("Error while getting key %v", err)
		} else {
			controller.Password = key
			flag = true
		}
	} else {
		flag = true
	}
	if flag {
		if controller.Token == "" {
			log.Info("Token is not there for ", controller.ControllerName, " getting it")
			token, err := appConfig.loginGateway(controller.MacAddress, controller.Password)
			if err != nil {
				log.Error(err)
			}
			return token
		}
	}

	return ""
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
	if success, ok := parsed["success"].(map[string]interface{}); ok {
		if data, ok := success["data"].(map[string]interface{}); ok {
			if token, ok := data["Token"].(string); ok {
				return token, nil
			}
		}
	}

	return "", fmt.Errorf("token not found in response")
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
