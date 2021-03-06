package ovn

import (
	"encoding/json"
	"fmt"
	util "github.com/openvswitch/ovn-kubernetes/go-controller/pkg/util"
	"github.com/sirupsen/logrus"
	"hash/fnv"
	"strconv"
	"strings"
)

// hash the provided input to make it a valid addressSet name.
func hashedAddressSet(s string) string {
	h := fnv.New64a()
	_, err := h.Write([]byte(s))
	if err != nil {
		logrus.Errorf("failed to hash %s", s)
	}
	hashString := strconv.FormatUint(h.Sum64(), 10)
	return fmt.Sprintf("a%s", hashString)
}

// forEachAddressSetUnhashedName will pass the unhashedName, namespaceName and
// the first suffix in the name to the 'iteratorFn' for every address_set in
// OVN. (Each unhashed name for an addressSet can be of the form
// namespaceName.suffix1.suffix2. .suffixN)
func (oc *Controller) forEachAddressSetUnhashedName(iteratorFn func(
	string, string, string)) error {
	output, stderr, err := util.RunOVNNbctlUnix("--data=bare", "--no-heading",
		"--columns=external_ids", "find", "address_set")
	if err != nil {
		logrus.Errorf("Error in obtaining list of address sets from OVN: "+
			"stdout: %q, stderr: %q err: %v", output, stderr, err)
		return err
	}
	for _, addrSet := range strings.Fields(output) {
		if !strings.HasPrefix(addrSet, "name=") {
			continue
		}
		addrSetName := addrSet[5:]
		names := strings.Split(addrSetName, ".")
		addrSetNamespace := names[0]
		nameSuffix := ""
		if len(names) >= 2 {
			nameSuffix = names[1]
		}
		iteratorFn(addrSetName, addrSetNamespace, nameSuffix)
	}
	return nil
}

func (oc *Controller) setAddressSet(hashName string, addresses []string) {
	logrus.Debugf("setAddressSet for %s with %s", hashName, addresses)
	if len(addresses) == 0 {
		_, stderr, err := util.RunOVNNbctlUnix("clear", "address_set",
			hashName, "addresses")
		if err != nil {
			logrus.Errorf("failed to clear address_set, stderr: %q (%v)",
				stderr, err)
		}
		return
	}

	ips := strings.Join(addresses, " ")
	_, stderr, err := util.RunOVNNbctlUnix("set", "address_set",
		hashName, fmt.Sprintf("addresses=%s", ips))
	if err != nil {
		logrus.Errorf("failed to set address_set, stderr: %q (%v)",
			stderr, err)
	}
}

func (oc *Controller) createAddressSet(name string, hashName string,
	addresses []string) {
	logrus.Debugf("createAddressSet with %s and %s", name, addresses)
	addressSet, stderr, err := util.RunOVNNbctlUnix("--data=bare",
		"--no-heading", "--columns=_uuid", "find", "address_set",
		fmt.Sprintf("name=%s", hashName))
	if err != nil {
		logrus.Errorf("find failed to get address set, stderr: %q (%v)",
			stderr, err)
		return
	}

	// addressSet has already been created in the database and nothing to set.
	if addressSet != "" && len(addresses) == 0 {
		_, stderr, err = util.RunOVNNbctlUnix("clear", "address_set",
			hashName, "addresses")
		if err != nil {
			logrus.Errorf("failed to clear address_set, stderr: %q (%v)",
				stderr, err)
		}
		return
	}

	ips := strings.Join(addresses, " ")

	// An addressSet has already been created. Just set addresses.
	if addressSet != "" {
		// Set the addresses
		_, stderr, err = util.RunOVNNbctlUnix("set", "address_set",
			hashName, fmt.Sprintf("addresses=%s", ips))
		if err != nil {
			logrus.Errorf("failed to set address_set, stderr: %q (%v)",
				stderr, err)
		}
		return
	}

	// addressSet has not been created yet. Create it.
	if len(addresses) == 0 {
		_, stderr, err = util.RunOVNNbctlUnix("create", "address_set",
			fmt.Sprintf("name=%s", hashName),
			fmt.Sprintf("external-ids:name=%s", name))
	} else {
		_, stderr, err = util.RunOVNNbctlUnix("create", "address_set",
			fmt.Sprintf("name=%s", hashName),
			fmt.Sprintf("external-ids:name=%s", name),
			fmt.Sprintf("addresses=%s", ips))
	}
	if err != nil {
		logrus.Errorf("failed to create address_set %s, stderr: %q (%v)",
			name, stderr, err)
	}
}

func (oc *Controller) deleteAddressSet(hashName string) {
	logrus.Debugf("deleteAddressSet %s", hashName)

	_, stderr, err := util.RunOVNNbctlUnix("--if-exists", "destroy",
		"address_set", hashName)
	if err != nil {
		logrus.Errorf("failed to destroy address set %s, stderr: %q, (%v)",
			hashName, stderr, err)
		return
	}
}

func (oc *Controller) getIPFromOvnAnnotation(ovnAnnotation string) string {
	if ovnAnnotation == "" {
		return ""
	}

	var ovnAnnotationMap map[string]string
	err := json.Unmarshal([]byte(ovnAnnotation), &ovnAnnotationMap)
	if err != nil {
		logrus.Errorf("Error in json unmarshaling ovn annotation "+
			"(%v)", err)
		return ""
	}

	ipAddressMask := strings.Split(ovnAnnotationMap["ip_address"], "/")
	if len(ipAddressMask) != 2 {
		logrus.Errorf("Error in splitting ip address")
		return ""
	}

	return ipAddressMask[0]
}

func (oc *Controller) getMacFromOvnAnnotation(ovnAnnotation string) string {
	if ovnAnnotation == "" {
		return ""
	}

	var ovnAnnotationMap map[string]string
	err := json.Unmarshal([]byte(ovnAnnotation), &ovnAnnotationMap)
	if err != nil {
		logrus.Errorf("Error in json unmarshaling ovn annotation "+
			"(%v)", err)
		return ""
	}

	return ovnAnnotationMap["mac_address"]
}

func stringSliceMembership(slice []string, key string) bool {
	for _, val := range slice {
		if val == key {
			return true
		}
	}
	return false
}
