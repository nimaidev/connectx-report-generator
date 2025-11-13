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
	// for {
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
		//Start in a different thread
		go appConfig.StartReportGenerationForController(controller, db)
	}
	// sleep for 30s
	// time.Sleep(30 * time.Second)
	// }
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
		isSuccess := appConfig.sendHeartBeat(controller, db)
		if isSuccess {
			appConfig.saveControllerData(controller, db)
		}
	}
}

func (appConfig AppConfig) performAuthOperationIfRequired(controller ControllerMaster, db *gorm.DB) {

	if controller.Password == "" {
		log.Info("Getting secret key for: " + controller.MacAddress)

		// Retry until we get a valid secret key
		var password string
		var err error
		maxRetries := 10
		retryDelay := 5 * time.Second

		for attempt := 1; attempt <= maxRetries; attempt++ {
			log.WithFields(logrus.Fields{
				"attempt":     attempt,
				"mac_address": controller.MacAddress,
			}).Info("Attempting to get secret key")

			password, err = appConfig.GetSecretKey(controller.MacAddress)
			if err != nil {
				log.WithFields(logrus.Fields{
					"attempt":     attempt,
					"error":       err,
					"mac_address": controller.MacAddress,
				}).Warn("Failed to get secret key, retrying...")

				if attempt < maxRetries {
					time.Sleep(retryDelay)
				}
				continue
			}

			if password != "" {
				log.WithFields(logrus.Fields{
					"attempt":     attempt,
					"mac_address": controller.MacAddress,
				}).Info("Secret key retrieved successfully")
				break
			} else {
				log.WithFields(logrus.Fields{
					"attempt":     attempt,
					"mac_address": controller.MacAddress,
				}).Warn("Empty secret key received, retrying...")

				if attempt < maxRetries {
					time.Sleep(retryDelay)
				}
			}
		}

		if password == "" {
			log.WithField("mac_address", controller.MacAddress).Error("Failed to get secret key after all retries")
			return
		}

		// Save the password once we have it
		controller.Password = password
		err = appConfig.saveControllerData(controller, db)
		if err != nil {
			log.Errorf("Error saving controller data: %v", err)
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
		log.WithField("controller_name", controller.ControllerName).Info("Controller details saved successfully")
		return nil
	}
}

func (appConfig AppConfig) sendHeartBeat(controller ControllerMaster, db *gorm.DB) bool {
	url := fmt.Sprintf("%s/api/gms/sync/v1/to-controller", appConfig.ServerUrl)
	log.Info("URL for heartbeat: ", url)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Error("Error: ", err)
		return false
	}
	req.Header.Set("Authorization", "Bearer "+controller.Token)
	req.Header.Set("seqId", "-1")
	req.Header.Set("isRebooted", "false")

	res, err := GetHttpClient().Do(req)
	if err != nil {
		log.Errorf("HTTP Error : %v", err)
	}

	if res.StatusCode == http.StatusUnauthorized {
		log.Error("Gateway got logged out")
		controller.Token = ""
		appConfig.performAuthOperationIfRequired(controller, db)
		return false
	}

	if res.StatusCode == http.StatusOK {
		log.Info("Heartbeat for " + controller.MacAddress + " sent successfully")
		return true
	}

	//read the response for debug
	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		log.Errorf("failed to read response: %v", err)
	}
	log.WithField("response", string(resBody)).Info("Got the response")
	return false

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
