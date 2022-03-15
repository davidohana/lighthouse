package controller

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/submariner-io/lighthouse/pkg/lhutil"
	mcsv1a1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

func PrettyPrint(obj interface{}) string {
	jsonBytes, err := json.MarshalIndent(obj, "", "\t")
	if err != nil {
		return fmt.Sprintf("%#v", obj)
	}

	return string(jsonBytes)
}

func GetObjectNameWithClusterID(name, namespace, clusterID string) string {
	return lhutil.GenerateObjectName(name, namespace, clusterID)
}

func serviceExportConditionEqual(c1, c2 *mcsv1a1.ServiceExportCondition) bool {
	return c1.Type == c2.Type && c1.Status == c2.Status && reflect.DeepEqual(c1.Reason, c2.Reason) &&
		reflect.DeepEqual(c1.Message, c2.Message)
}
