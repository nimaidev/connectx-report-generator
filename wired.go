package main

import "time"

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
