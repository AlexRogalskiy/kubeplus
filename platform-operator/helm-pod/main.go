package main

import (
	restful "github.com/emicklei/go-restful"
	"time"
	"fmt"
	"bytes"
	"net/http"
	"net/url"
	"io/ioutil"
	"k8s.io/client-go/rest"
	//"k8s.io/client-go/kubernetes"
	//"os/exec"
	"strings"
	"strconv"
	//"sync"

	"os"
	"k8s.io/client-go/dynamic"
	"k8s.io/apimachinery/pkg/runtime/schema"

	//"k8s.io/cli-runtime/pkg/genericclioptions"
	//restclient "k8s.io/client-go/rest"
	//"k8s.io/kubectl/pkg/util/templates"
	//"k8s.io/kubernetes/pkg/kubectl/cmd/exec"

	//"github.com/golang/glog"
	
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/remotecommand"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	platformworkflowclientset "github.com/cloud-ark/kubeplus/platform-operator/pkg/client/clientset/versioned"
	//platformworkflowv1alpha1 "github.com/cloud-ark/kubeplus/platform-operator/pkg/apis/workflowcontroller/v1alpha1"
)

type fileSpec struct {
	PodNamespace string
	PodName      string
	File         string
}

type kindDetails struct {
	Kind string
	Version string
	Group string
	Plural string
}

var (
	kubeClient kubernetes.Interface
	dynamicClient dynamic.Interface
	cfg *rest.Config
	err error
	kindDetailsMap map[string]kindDetails
	KUBEPLUS_DEPLOYMENT string
	CMD_RUNNER_CONTAINER string
	KUBEPLUS_NAMESPACE string
	HELMER_PORT string
)

func init() {
	cfg, err = rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	kubeClient = kubernetes.NewForConfigOrDie(cfg)
	dynamicClient, err = dynamic.NewForConfig(cfg)
	kindDetailsMap = make(map[string]kindDetails, 0)
	KUBEPLUS_DEPLOYMENT = "kubeplus-deployment"
	CMD_RUNNER_CONTAINER = "helmer"
	KUBEPLUS_NAMESPACE = getNamespace()
	HELMER_PORT = "8090"
}

func main() {
	fmt.Printf("Before registering...\n")
	register()
	fmt.Printf("After registering...\n")
	for {
		time.Sleep(60 * time.Second)
	}
}

func register() {
	ws := new(restful.WebService)
	ws.
		Path("/apis/kubeplus").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML)

	ws.Route(ws.GET("/deploy").To(deployChart).
		Doc("Deploy chart").
		Param(ws.PathParameter("customresource", "name of the customresource").DataType("string")))

	ws.Route(ws.GET("/metrics").To(getMetrics).
		Doc("Get Metrics"))

	ws.Route(ws.GET("/testChartDeployment").To(testChartDeployment).
		Doc("Test Chart Deployment"))

	ws.Route(ws.GET("/getPlural").To(getPlural).
		Doc("Get Plural"))

	ws.Route(ws.GET("/checkResource").To(checkResource).
		Doc("Check Resource"))

	ws.Route(ws.GET("/annotatecrd").To(annotateCRD).
		Doc("Annotate CRD"))

	ws.Route(ws.GET("/deletecrdinstances").To(deleteCRDInstances).
		Doc("Delete CRD Instances"))

	ws.Route(ws.GET("/getchartvalues").To(getChartValues).
		Doc("Get Chart Values"))

	restful.Add(ws)
	http.ListenAndServe(":" + HELMER_PORT, nil)
	fmt.Printf("Listening on port " + HELMER_PORT + " ...")
	fmt.Printf("Done installing helmer paths...")
}

func getNamespace() string {
	filePath := "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
	content, err := ioutil.ReadFile(filePath)
    if err != nil {
     	fmt.Printf("Namespace file reading error:%v\n", err)
    }
    ns := string(content)
    ns = strings.TrimSpace(ns)
    fmt.Printf("Helmer NS:%s\n", ns)
    return ns
}

func getChartValues(request *restful.Request, response *restful.Response) {

    platformWorkflowName := request.QueryParameter("platformworkflow")
	namespace := request.QueryParameter("namespace")
	fmt.Printf("PlatformWorkflowName:%s\n", platformWorkflowName)
	fmt.Printf("Namespace:%s\n", namespace)

 	var valuesToReturn string

	if platformWorkflowName != "" {
		config, err := rest.InClusterConfig()
		if err != nil {
			panic(err.Error())
		}

		var sampleclientset platformworkflowclientset.Interface
		sampleclientset = platformworkflowclientset.NewForConfigOrDie(config)

		platformWorkflow1, err := sampleclientset.WorkflowsV1alpha1().ResourceCompositions(namespace).Get(platformWorkflowName, metav1.GetOptions{})
		//fmt.Printf("PlatformWorkflow:%v\n", platformWorkflow1)
		if err != nil {
			fmt.Errorf("Error:%s\n", err)
		}

	    customAPI := platformWorkflow1.Spec.NewResource
    	//for _, customAPI := range customAPIs {
    		kind := customAPI.Resource.Kind
    		group := customAPI.Resource.Group
    		version := customAPI.Resource.Version
    		plural := customAPI.Resource.Plural
    		chartURL := customAPI.ChartURL
    		chartName := customAPI.ChartName
 			fmt.Printf("Kind:%s, Group:%s, Version:%s, Plural:%s, ChartURL:%s ChartName:%s\n", kind, group, version, plural, chartURL, chartName)

 			cmdRunnerPod := getKubePlusPod()
 			if cmdRunnerPod == "" {
 				fmt.Printf("Command runner Pod name could not be determined.. cannot continue.")
 				valuesToReturn = ""
 			}
 			parsedChartName := downloadUntarChartandGetName(chartURL, cmdRunnerPod, namespace)
 			chartValuesPath := "/" + parsedChartName + "/values.yaml"
 			fmt.Printf("Chart Values Path:%s\n",chartValuesPath)
 			readCmd := "more " + chartValuesPath
 			fmt.Printf("More cmd:%s\n", readCmd)
 			_, valuesToReturn = executeExecCall(cmdRunnerPod, namespace, readCmd)
 			fmt.Printf("valuesToReturn:%v\n",valuesToReturn)
		}

	response.Write([]byte(valuesToReturn))
}

func getKubePlusPod() string {
	podName := ""
	/*
	deploymentObj, err1 := kubeClient.AppsV1().Deployments(KUBEPLUS_NAMESPACE).Get(KUBEPLUS_DEPLOYMENT, metav1.GetOptions{})
	if err1 != nil {
		fmt.Printf("Error:%v\n", err1)
		return podName
	}*/
	replicaSetList, err2 := kubeClient.AppsV1().ReplicaSets(KUBEPLUS_NAMESPACE).List(metav1.ListOptions{})
	if err2 != nil {
		fmt.Printf("Error:%v\n", err2)
		return podName
	}
	replicaSetName := ""
	for _, repSetObj := range replicaSetList.Items {
		ownerRefObj := repSetObj.ObjectMeta.OwnerReferences[0]
		depOwnerName := ownerRefObj.Name
		if depOwnerName == KUBEPLUS_DEPLOYMENT {
			replicaSetName = repSetObj.ObjectMeta.Name
			//fmt.Printf("DepOwnerName:%s, RSSetName:%s\n", depOwnerName, replicaSetName)
			break
		}
	}

	podList, err3 := kubeClient.CoreV1().Pods(KUBEPLUS_NAMESPACE).List(metav1.ListOptions{})
	if err3 != nil {
		fmt.Printf("Error:%v\n", err3)
		return podName
	}
	for _, podObj := range podList.Items {
		ownerRefObj := podObj.ObjectMeta.OwnerReferences[0]
		podOwnerName := ownerRefObj.Name
		if podOwnerName == replicaSetName {
			podName = podObj.ObjectMeta.Name
			//fmt.Printf("RSSetName:%s, PodName:%s\n", replicaSetName, podName)
			break
		}
	}
	return podName
}

func deleteCRDInstances(request *restful.Request, response *restful.Response) {

	//sync.Mutex.Lock()
	fmt.Printf("Inside deleteCRDInstances...\n")
	kind := request.QueryParameter("kind")
	group := request.QueryParameter("group")
	version := request.QueryParameter("version")
	plural := request.QueryParameter("plural")
	namespace := request.QueryParameter("namespace")
	crName := request.QueryParameter("instance")
	fmt.Printf("Kind:%s\n", kind)
	fmt.Printf("Group:%s\n", group)
	fmt.Printf("Version:%s\n", version)
	fmt.Printf("Plural:%s\n", plural)
	fmt.Printf("Namespace:%s\n", namespace)
	fmt.Printf("CRName:%s\n", crName)

	apiVersion := group + "/" + version

	fmt.Printf("APIVersion:%s\n", apiVersion)
	
	ownerRes := schema.GroupVersionResource{Group: group,
									 		Version: version,
									   		Resource: plural}

	fmt.Printf("GVR:%v\n", ownerRes)

	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	dynamicClient, _ := dynamic.NewForConfig(config)

	crdObjList, err := dynamicClient.Resource(ownerRes).Namespace(namespace).List(metav1.ListOptions{})
	if err != nil {
		fmt.Printf("Error:%v\n...checking in non-namespace", err)
		crdObjList, err = dynamicClient.Resource(ownerRes).List(metav1.ListOptions{})
		if err != nil {
			fmt.Printf("Error:%v\n", err)
		}
	}
	//fmt.Printf("CRDObjList:%v\n", crdObjList)

	for _, instanceObj := range crdObjList.Items {
		objData := instanceObj.UnstructuredContent()
		//mapval, ok1 := lhsContent.(map[string]interface{})
		//if ok1 {
		objName := instanceObj.GetName()
		//fmt.Printf("Instance Name:%s\n", objName)

		if crName != "" && objName != crName {
			continue;
		}

		//fmt.Printf("objData:%v\n", objData)
		status := objData["status"]
		//fmt.Printf("Status:%v\n", status)
		if status != nil {
			helmreleaseNS, helmrelease := getHelmReleaseName(status)
			fmt.Printf("Helm release:%s, %s\n", helmreleaseNS, helmrelease)
			if helmreleaseNS != "" && helmrelease != "" {
				ok := deleteHelmRelease(helmreleaseNS, helmrelease)
				if ok {
					fmt.Printf("Helm release deleted...\n")
					fmt.Printf("Deleting the namespace...\n")
					
					// The namespace created to deploy the Helm chart
					// needs to be deleted.
					namespaceDeleteCmd := "./root/kubectl delete ns " + helmreleaseNS
					cmdRunnerPod := getKubePlusPod()
					_, execOutput := executeExecCall(cmdRunnerPod, KUBEPLUS_NAMESPACE, namespaceDeleteCmd)
                                	fmt.Printf("Output of Delete NS Cmd:%v\n", execOutput)

					if crName == "" {
						fmt.Printf("Deleting the object %s\n", objName)
						dynamicClient.Resource(ownerRes).Namespace(namespace).Delete(objName, &metav1.DeleteOptions{})
					}
				}
			}
		}
	}

	if crName == "" { //crName == "" means that we received request to delete all objects
		lowercaseKind := strings.ToLower(kind)
		configMapName := lowercaseKind + "-usage"
		// Delete the usage configmap
		fmt.Printf("Deleting the usage configmap:%s\n", configMapName)
		kubeClient.CoreV1().ConfigMaps(namespace).Delete(configMapName, &metav1.DeleteOptions{})
		fmt.Println("Done deleting CRD Instances..")
	}
	//sync.mutex.Unlock()
}

func deleteHelmRelease(helmreleaseNS, helmrelease string) bool {
	fmt.Printf("Helm release:%s\n", helmrelease)
	cmd := "./root/helm delete " + helmrelease + " -n " + helmreleaseNS
	fmt.Printf("Helm delete cmd:%s\n", cmd)
	var output string 
	namespace := KUBEPLUS_NAMESPACE // NAMEspace for the KubePlus Pod
	cmdRunnerPod := getKubePlusPod()
	ok, output := executeExecCall(cmdRunnerPod, namespace, cmd)
	fmt.Printf("Helm delete o/p:%v\n", output)
	return ok
}

func annotateCRD(request *restful.Request, response *restful.Response) {
	fmt.Printf("Inside annotateCRD...\n")
	kind := request.QueryParameter("kind")
	group := request.QueryParameter("group")
	plural := request.QueryParameter("plural")
	chartkinds := request.QueryParameter("chartkinds")	
	//fmt.Printf("Kind:%s\n", kind)
	//fmt.Printf("Group:%s\n", group)
	//fmt.Printf("Plural:%s\n", plural)
	//fmt.Printf("Chart Kinds:%s\n", chartkinds)
	chartkinds = strings.Replace(chartkinds, "-", ";", 1)

 	cmdRunnerPod := getKubePlusPod()

 	namespace := KUBEPLUS_NAMESPACE

	//kubectl annotate crd mysqlservices.platformapi.kubeplus resource/annotation-relationship="on:MysqlCluster; Secret, key:meta.helm.sh/release-name, value:INSTANCE.metadata.name"

 	fqcrd := plural + "." + group
 	fmt.Printf("FQCRD:%s\n", fqcrd)
 	lowercaseKind := strings.ToLower(kind)
 	annotationValue := lowercaseKind + "-INSTANCE.metadata.name"
 	fmt.Printf("Annotation value:%s\n", annotationValue)
 	annotationString := " resource/annotation-relationship=\"" + "on:" + chartkinds + ", key:meta.helm.sh/release-name, value:" + annotationValue + "\""
 	fmt.Printf("Annotation String:%s\n", annotationString)
	cmd := "./root/kubectl annotate crd " + fqcrd + annotationString
	fmt.Printf("API resources cmd:%s\n", cmd)
	var ok bool
	var output string 
	for {
		ok, output = executeExecCall(cmdRunnerPod, namespace, cmd)
		fmt.Printf("CRD annotate o/p:%v\n", output)
		if !ok {
			time.Sleep(1 * time.Second)
		} else {
			break
		}
	}
}

func checkResource(request *restful.Request, response *restful.Response) {
	fmt.Printf("Inside checkResource...\n")

	kind := request.QueryParameter("kind")
	plural := request.QueryParameter("plural")

 	cmdRunnerPod := getKubePlusPod()

 	namespace := KUBEPLUS_NAMESPACE

	cmd := "./root/kubectl api-resources " //| grep " + kind + " | grep " + group + " | awk '{print $1}' " 
	fmt.Printf("API resources cmd:%s\n", cmd)
	_, output := executeExecCall(cmdRunnerPod, namespace, cmd)

	failed := ""
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		parts := strings.Split(line, " ")
		nonEmptySlice := make([]string,0)
		for _, p := range parts {
			if p != "" && p != " " {
				nonEmptySlice = append(nonEmptySlice, p)
			}
		}
		if len(nonEmptySlice) > 0 {
			existingKind := nonEmptySlice[len(nonEmptySlice)-1]
			existingKind = strings.TrimSuffix(existingKind, "\n")
			//fmt.Printf("ExistingKind:%s\n", existingKind)
			if kind == existingKind {
				failed = kind
				break
			}
			existingPlural := nonEmptySlice[0]
			existingPlural = strings.TrimSuffix(existingPlural, "\n")
			//fmt.Printf("ExistingKind:%s\n", existingKind)
			if plural == existingPlural {
				failed = plural
			}
		}
	}

	//fmt.Printf("Plural to return:%s\n", string(pluralToReturn))
	response.Write([]byte(failed))
}


func getPlural(request *restful.Request, response *restful.Response) {
	fmt.Printf("Inside getPlural...\n")

	kind := request.QueryParameter("kind")
	//group := request.QueryParameter("group")
	//fmt.Printf("Kind:%s\n", kind)
	//fmt.Printf("Group:%s\n", group)

 	cmdRunnerPod := getKubePlusPod()

 	namespace := KUBEPLUS_NAMESPACE

	cmd := "./root/kubectl api-resources " //| grep " + kind + " | grep " + group + " | awk '{print $1}' " 
	fmt.Printf("API resources cmd:%s\n", cmd)
	_, output := executeExecCall(cmdRunnerPod, namespace, cmd)

	pluralToReturn := ""
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		parts := strings.Split(line, " ")
		nonEmptySlice := make([]string,0)
		for _, p := range parts {
			if p != "" && p != " " {
				nonEmptySlice = append(nonEmptySlice, p)
			}
		}
		if len(nonEmptySlice) > 0 {
			existingKind := nonEmptySlice[len(nonEmptySlice)-1]
			existingKind = strings.TrimSuffix(existingKind, "\n")
			//fmt.Printf("ExistingKind:%s\n", existingKind)
			if kind == existingKind {
				pluralToReturn = nonEmptySlice[0]
				break
			}
		}
	}

	//fmt.Printf("Plural to return:%s\n", string(pluralToReturn))

	response.Write([]byte(pluralToReturn))
}

func getMetrics(request *restful.Request, response *restful.Response) {
	fmt.Printf("Inside getMetrics...\n")

	metricsToReturn := ""
 	cmdRunnerPod := getKubePlusPod()

	customresource := request.QueryParameter("instance")
	kind := request.QueryParameter("kind")
	namespace := request.QueryParameter("namespace")
	fmt.Printf("Custom Resource:%s\n", customresource)
	fmt.Printf("Kind:%s\n", kind)
	fmt.Printf("Namespace:%s\n", namespace)

	/*helmrelease := getReleaseName(kind, customresource, namespace)
	fmt.Printf("Helm release3:%s\n", helmrelease)
	if helmrelease != "" {
		metricsCmd := "./root/kubectl metrics helmrelease " + helmrelease + " -o prometheus " 
		fmt.Printf("metrics cmd:%s\n", metricsCmd)
		_, metricsToReturn = executeExecCall(cmdRunnerPod, namespace, metricsCmd)
	} else {*/
		config, _ := rest.InClusterConfig()
		var sampleclientset platformworkflowclientset.Interface
		sampleclientset = platformworkflowclientset.NewForConfigOrDie(config)

		resourceMonitors, err := sampleclientset.WorkflowsV1alpha1().ResourceMonitors(KUBEPLUS_NAMESPACE).List(metav1.ListOptions{})
		//followConnections := ""
		for _, resMonitor := range resourceMonitors.Items {
			fmt.Printf("ResourceMonitor:%v\n", resMonitor)
			if err != nil {
				fmt.Errorf("Error:%s\n", err)
			}
			reskind := resMonitor.Spec.Resource.Kind
			resgroup := resMonitor.Spec.Resource.Group
			resversion := resMonitor.Spec.Resource.Version
			resplural := resMonitor.Spec.Resource.Plural
			relationshipToMonitor := resMonitor.Spec.MonitorRelationships
			fmt.Printf("Kind:%s\n", reskind)
			fmt.Printf("Group:%s\n", resgroup)    		
			fmt.Printf("Version:%s\n", resversion)
			fmt.Printf("Plural:%s\n", resplural)
			fmt.Printf("RelationshipToMonitor:%s\n", relationshipToMonitor)

			if relationshipToMonitor != "all" && relationshipToMonitor != "owner" {
				metricsToReturn = "Value " + relationshipToMonitor + " not supported. Valid values: all, owner"
				break
			}
			if reskind == kind {
				if relationshipToMonitor == "all" {
					//followConnections = " --follow-connections"
				}
			}
		}
		metricsCmd := "./root/kubectl metrics " + kind + " " + customresource + " " + namespace + " -o prometheus " //+ followConnections
		fmt.Printf("metrics cmd:%s\n", metricsCmd)
		_, metricsToReturn = executeExecCall(cmdRunnerPod, namespace, metricsCmd)
	//}
 	/*cpPluginsCmd := "cp /plugins/* bin/"
	fmt.Printf("cp plugins cmd:%s\n", cpPluginsCmd)
	executeExecCall(cmdRunnerPod, namespace, cpPluginsCmd)
	*/
	/*if ok {
		fmt.Printf("%v\n", helmreleaseMetrics)
		// WIP - convert metrics from helmrelease to custom resource
		//prometheusMetrics := getPrometheusMetrics(kind, customresource, namespace, helmreleaseMetrics)
		response.Write([]byte(helmreleaseMetrics))
	} else {
		response.Write([]byte{})
	}*/
	response.Write([]byte(metricsToReturn))
}

/*
func getPrometheusMetrics(kind, customresource, namespace, helmreleaseMetrics string) string {
	var metrics_helm_release map[string]interface{}
	err := json.Unmarshal([]byte(helmreleaseMetrics), &metrics_helm_release)
	if err != nil {
    	panic(err)
	}

	cpu = metrics_helm_release["cpu"]
	memory = metrics_helm_release["memory"]
	storage = metrics_helm_release["storage"]
	num_of_pods = metrics_helm_release["num_of_pods"]
	num_of_containers = metrics_helm_release["num_of_containers"]

	millis := time.Now()
    cpuMetrics = "cpu{kind="+kind+",customresoure="+customresource+",namespace="+namespace+"} " + str(cpu) + " "  + str(millis)
    memoryMetrics = "memory{kind="+kind+",customresoure="+customresource+",namespace="+namespace+"} " + str(memory) + " " + str(millis)
    storageMetrics = "storage{kind="+kind+",customresoure="+customresource+",namespace="+namespace+"} " + str(storage) + " " + str(millis)

	//numOfPods = 'pods{helmrelease="'+release_name+'"} ' + str(num_of_pods) + ' ' + str(millis)
	//numOfContainers = 'containers{helmrelease="'+release_name+'"} ' + str(num_of_containers) + ' ' + str(millis)
	metricsToReturn = cpuMetrics + "\n" + memoryMetrics + "\n" + storageMetrics + "\n" + numOfPods + "\n" + numOfContainers
	return metricsToReturn
}
*/

func getReleaseName(kind, customresource, namespace string) (string, string) {
	helmrelease := ""
	helmreleaseNS := ""
	fmt.Printf("Kind:%s, Instance:%s, Namespace:%s\n", kind, customresource, namespace)
	derefstring := kind + ":" + customresource
	fmt.Printf("De-ref string:%s\n", derefstring)
	kinddetails, ok := kindDetailsMap[derefstring]
	if ok {
		fmt.Printf("Kinddetails:%v\n", kinddetails)
		group := kinddetails.Group
		version := kinddetails.Version
		plural := kinddetails.Plural

		res := schema.GroupVersionResource{Group: group,
										   Version: version,
										   Resource: plural}

		obj, _ := dynamicClient.Resource(res).Namespace(namespace).Get(customresource, metav1.GetOptions{})
		objData := obj.UnstructuredContent()
		fmt.Printf("objData:%v\n", objData)
		status := objData["status"]
		//fmt.Printf("Status:%v\n", status)
		helmreleaseNS, helmrelease = getHelmReleaseName(status)
		fmt.Printf("Helm release2:%s\n", helmrelease)
	}
	return helmreleaseNS, helmrelease
}

func getHelmReleaseName(object interface{}) (string, string) {
	helmrelease := ""
	helmreleaseNS := ""
	helmreleaseName := ""
	status := object.(map[string]interface{})
	for key, element := range status {
		//fmt.Printf("Key:%s\n",key)
		key = strings.TrimSpace(key)
		if key == "helmrelease" {
			helmrelease = element.(string)
			fmt.Printf("Helm release1:%s\n", helmrelease)
			parts := strings.Split(helmrelease,":")
			helmreleaseNS = strings.TrimSpace(parts[0])
			helmreleaseName = strings.TrimSpace(parts[1])
			break
		}
	}
	return helmreleaseNS, helmreleaseName
}

func downloadUntarChartandGetName(chartURL, cmdRunnerPod, namespace string) string {
	fmt.Printf("Inside downloadUntarChartandGetName\n")

	parsedChartName := ""
 	if !strings.Contains(chartURL, "file:///") {
 		// Download and untar the chart
 		parsedChartName = downloadChart(chartURL, cmdRunnerPod, namespace)
	} else {
		// Untar the chart
		parts := strings.Split(chartURL, "file:///")
		charttgz := strings.TrimSpace(parts[1])
		fmt.Printf("Chart tgz:%s\n",charttgz)
		parsedChartName = untarChart(charttgz, cmdRunnerPod, namespace)
	}
	return parsedChartName
}

// ./kubectl exec helmer-677f87c67f-xvzz6 -- ./root/helm install moodle-operator-chart
// curl -v "10.0.9.208/kubeplus/deploy?platformworkflow=moodle1-workflow&customresource=mystack1&namespace=default"
// curl -v "10.0.2.202/kubeplus/deploy?platformworkflow=mysqlcluster&customresource=stack1&namespace=default"
// ./kubectl exec kubeplus -c helmer -- sh -c "export KUBEPLUS_HOME=/; export PATH=$KUBEPLUS_HOME/plugins/:/root:$PATH; kubectl metrics helmrelease falling-aardvark"
// curl -v "10.0.3.244:90/apis/platform-as-code/metrics?kind=MysqlClusterStack&customresource=stack1&namespace=default"
func deployChart(request *restful.Request, response *restful.Response) {
	fmt.Printf("Inside deployChart...\n")

    platformWorkflowName := request.QueryParameter("platformworkflow")
	customresource := request.QueryParameter("customresource")
	namespace := request.QueryParameter("namespace")
	encodedoverrides := request.QueryParameter("overrides")
	overrides, err := url.QueryUnescape(encodedoverrides)
	if err != nil {
		fmt.Printf("Error encountered in decoding overrides:%v\n", err)
		fmt.Printf("Not continuing...")
		return
	}
	dryrun := request.QueryParameter("dryrun")
	fmt.Printf("PlatformWorkflowName:%s\n", platformWorkflowName)
	fmt.Printf("Custom Resource:%s\n", customresource)
	fmt.Printf("Resource Composition:%s\n", platformWorkflowName)
	fmt.Printf("Namespace:%s\n", namespace)
	fmt.Printf("Overrides:%s\n", overrides)
	fmt.Printf("Dryrun:%s\n", dryrun)

	kinds := make([]string, 0)
	ok := false
	execOutput := ""

	if platformWorkflowName != "" {
		config, err := rest.InClusterConfig()
		if err != nil {
			panic(err.Error())
		}

		var sampleclientset platformworkflowclientset.Interface
		sampleclientset = platformworkflowclientset.NewForConfigOrDie(config)

		resourceCompositionNS := KUBEPLUS_NAMESPACE
		platformWorkflow1, err := sampleclientset.WorkflowsV1alpha1().ResourceCompositions(resourceCompositionNS).Get(platformWorkflowName, metav1.GetOptions{})
		fmt.Printf("PlatformWorkflow:%v\n", platformWorkflow1)
		if err != nil {
			fmt.Errorf("Error:%s\n", err)
		}

	    customAPI := platformWorkflow1.Spec.NewResource
    	//for _, customAPI := range customAPIs {
    		kind := customAPI.Resource.Kind
    		group := customAPI.Resource.Group
    		version := customAPI.Resource.Version
    		plural := customAPI.Resource.Plural
    		chartURL := customAPI.ChartURL
    		chartName := customAPI.ChartName
 			fmt.Printf("Kind:%s, Group:%s, Version:%s, Plural:%s, ChartURL:%s\n ChartName:%s", kind, group, version, plural, chartURL, chartName)
 			
 			kinddetails := kindDetails{
 				Group: group,
 				Version: version,
 				Kind: kind,
 				Plural: plural,
 			}
 			kindDetailsMap[kind + ":" + customresource] = kinddetails

 			cmdRunnerPod := getKubePlusPod()
 			if chartURL != "" {

 			parsedChartName := downloadUntarChartandGetName(chartURL, cmdRunnerPod, namespace)

 				// 1. Download the chart
 				//parsedChartName := downloadChart(chartURL, cmdRunnerPod, namespace)

	 			// 5. Create overrides.yaml
	 			//overrides := getOverrides(kind, group, version, plural, customresource, namespace)
	 			chartDir := "/chart/" + parsedChartName
	 			fmt.Printf("Chart dir:%s\n", chartDir)
	 			overRidesFile := chartDir + "/overrides.yaml"
	 			fmt.Printf("Overrides file:%s\n", overRidesFile)
	 			os.Mkdir(chartDir, 0755)
	 			f, errf := os.Create(overRidesFile)
	 			if errf != nil {
	 				fmt.Errorf("Error:%s\n", errf)
	 			}
	 			f.WriteString(overrides)

	 			// 6. Install the Chart
	 			//fmt.Printf("ChartName to install:%s\n", chartName)
	 			lowercaseKind := strings.ToLower(kind)
	 			releaseName := lowercaseKind + "-" + customresource
	 			fmt.Printf("Release name:%s\n", releaseName)

				if dryrun == "" {
					createNSCmd := "./root/kubectl create ns " + customresource
					_, execOutput = executeExecCall(cmdRunnerPod, namespace, createNSCmd)
					fmt.Printf("Output of Create NS Cmd:%v\n", execOutput)

					annotateNSCmd := "./root/kubectl annotate namespace " + customresource + " meta.helm.sh/release-name=\"" + releaseName + "\""
					fmt.Printf("Annotation NS Cmd:%v\n", annotateNSCmd)
					_, execOutput = executeExecCall(cmdRunnerPod, namespace, annotateNSCmd)
					fmt.Printf("Output of Annotate NS Cmd:%v\n", execOutput)

				}

				// Install the Helm chart in the namespace that is created for that instance
	 			helmInstallCmd := "./root/helm install " + releaseName + " ./" + parsedChartName  + " -f " + overRidesFile + " -n " + customresource
	 			if dryrun != "" {
		 			helmInstallCmd = "./root/helm install " + releaseName + " ./" + parsedChartName  + " -f " + overRidesFile + " -n " + namespace + " --dry-run" 		
	 			}
	  			fmt.Printf("ABC helm install cmd:%s\n", helmInstallCmd)
	 			ok, execOutput = executeExecCall(cmdRunnerPod, namespace, helmInstallCmd)
	 			if ok {
	 				helmReleaseOP := execOutput
	 				lines := strings.Split(helmReleaseOP, "\n")
	 				for _, line := range lines {
	 					if dryrun == "" {
		 					present := strings.Contains(line, "NAME:")
		 					if present {
	 							parts := strings.Split(line, ":")
	 							for _, part := range parts {
		 							if part != "" && part != " " && part != "NAME" {
				 						releaseName := part
				 						fmt.Printf("ReleaseName:%s\n", releaseName)
		 								releaseName = strings.TrimSpace(releaseName)
		 								fmt.Printf("RN:%s\n", releaseName)
		 								go updateStatus(kind, group, version, plural, customresource, namespace, releaseName)
		 							}
	 							}
	 						}
	 					} else {
	 						present := strings.Contains(line, "kind:")
	 						if present {
	 							parts := strings.Split(line, ":")
	 							if len(parts) >= 2 {
	 								kindName := parts[1]
	 								kindName = strings.TrimSpace(kindName)
	 								kindName = strings.TrimSuffix(kindName, "\n")
	 								//fmt.Printf("Found Kind:%s\n", kindName)
	 								kinds = append(kinds, kindName)
	 							}
	 						}
	 					}
	 				}
	 			} else {
		 			go updateStatus(kind, group, version, plural, customresource, namespace, execOutput)
	 			}
	    	}
 		//}
	}
	if dryrun == "true" {
		fmt.Printf("DRY RUN - ABC:%s\n", dryrun)
		//fmt.Printf("Kinds:%v\n", kinds)
		if strings.Contains(execOutput, "Error") {
			fmt.Printf("ExecOutput:%s\n", execOutput)
			response.Write([]byte(execOutput))
		} else {
			// Appending the Namespace to the list of Kinds since we are creating NS corresponding to
			// each Helm release.
			kinds = append(kinds, "Namespace")
			kindsString := strings.Join(kinds, "-")
			fmt.Printf("KindString:%s\n", kindsString)
			response.Write([]byte(kindsString))
		}
	}
}

func testChartDeployment(request *restful.Request, response *restful.Response) {
	fmt.Printf("Inside testChartDeployment...\n")

	namespace := request.QueryParameter("namespace")
	kind := request.QueryParameter("kind")
	//chartName := request.QueryParameter("chartName")
	encodedChartURL := request.QueryParameter("chartURL")
	chartURL, _ := url.QueryUnescape(encodedChartURL)

 	cmdRunnerPod := getKubePlusPod()

  	//parsedChartName := downloadChart(chartURL, cmdRunnerPod, namespace)
 	parsedChartName := downloadUntarChartandGetName(chartURL, cmdRunnerPod, namespace)
 	releaseName := kind + "-" + parsedChartName

	helmInstallCmd := "./root/helm install " + releaseName + " ./" + parsedChartName  + " -n " + namespace + " --dry-run" 
	fmt.Printf("helm install cmd:%s\n", helmInstallCmd)
	_, execOutput := executeExecCall(cmdRunnerPod, namespace, helmInstallCmd)
	fmt.Printf("DRY RUN - DEF:%s\n", execOutput)
	response.Write([]byte(execOutput))
}

func downloadChart(chartURL, cmdRunnerPod, namespace string) string {
	 			// 1. Extract Chart Name
	 			lastIndexOfSlash := strings.LastIndex(chartURL, "/")
	 			chartName1 := chartURL[lastIndexOfSlash+1:]
	 			//fmt.Printf("ChartName1:%s\n", chartName1)
	 			parts := strings.Split(chartName1, "?")
	 			chartName2 := parts[0]
	 			//fmt.Printf("ChartName2:%s\n", chartName2)

	 			lsCmd := "ls -l "

                // 1. Remove previous instance of chart
                rmCmd := "rm -rf /" + chartName2
	 			fmt.Printf("rm cmd:%s\n", rmCmd)
	 			executeExecCall(cmdRunnerPod, namespace, rmCmd)
	 			executeExecCall(cmdRunnerPod, namespace, lsCmd)

	 			// 2. Download the Chart
	 			wgetCmd := "wget --no-check-certificate " + chartURL
	 			fmt.Printf("wget cmd:%s\n", wgetCmd)
	 			executeExecCall(cmdRunnerPod, namespace, wgetCmd)
	 			executeExecCall(cmdRunnerPod, namespace, lsCmd)

	 			// 3. Rename the Chart to a friendlier name
	 			mvCmd := "mv /" + chartName1 + " /" + chartName2
	 			fmt.Printf("mv cmd:%s\n", mvCmd)
	 			executeExecCall(cmdRunnerPod, namespace, mvCmd)
	 			executeExecCall(cmdRunnerPod, namespace, lsCmd)

	 			chartName := untarChart(chartName2, cmdRunnerPod, namespace)

	 			return chartName
}

func untarChart(chartName2, cmdRunnerPod, namespace string) string {
				fmt.Printf("Inside untarChart.")

	 			// 4. Untar the Chart file
	 			untarCmd := "tar -xvzf " + chartName2
	  			fmt.Printf("untar cmd:%s\n", untarCmd)
	 			_, op := executeExecCall(cmdRunnerPod, namespace, untarCmd)
	 			fmt.Printf("Untar output:%s",op)
	 			lines := strings.Split(op, "\n")
	 			chartName := ""
	 			parts := strings.Split(lines[0],"/")
	 			fmt.Printf("ABC:%v",parts)
	 			chartName = strings.TrimSpace(parts[0])
	 			fmt.Printf("Chart Name:%s\n", chartName)

	 			lsCmd := "ls -l "
	 			executeExecCall(cmdRunnerPod, namespace, lsCmd)
	 			return chartName
}

func updateStatus(kind, group, version, plural, instance, namespace, releaseName string) {

	res := schema.GroupVersionResource{Group: group,
									   Version: version,
									   Resource: plural}
	for {
		obj, err := dynamicClient.Resource(res).Namespace(namespace).Get(instance, metav1.GetOptions{})
		if err == nil {
			objData := obj.UnstructuredContent()
			helmrelease := make(map[string]interface{},0)
			// Helm release will be done in the new namespace that is created
			// corresponding to the customresource instance.
			helmrelease["helmrelease"] = instance + ":" + releaseName
			objData["status"] = helmrelease
			obj.SetUnstructuredContent(objData)
			dynamicClient.Resource(res).Namespace(namespace).Update(obj, metav1.UpdateOptions{})
			// break out of the for loop
			break 
		} else {
			time.Sleep(2 * time.Second)
		}
	}
	fmt.Printf("Done updating status of the CR instance:%s\n", instance)
}

// We cannot reach into the object to get the overrides as the object will not be
// created when deployChart will be called.
func getOverrides(kind, group, version, plural, instance, namespace string) string {
	var overrides string
	overrides = ""

	res := schema.GroupVersionResource{Group: group,
									   Version: version,
									   Resource: plural}

	obj, _ := dynamicClient.Resource(res).Namespace(namespace).Get(instance, metav1.GetOptions{})
	objData := obj.UnstructuredContent()
	fmt.Printf("objData:%v\n", objData)
	spec := objData["spec"]
	fmt.Printf("Spec:%v\n", spec)
	for key, element := range spec.(map[string]interface{}) {
		//fmt.Printf("Key:%s\n",key)
		elem, ok := element.(int64)
		if ok {
			strelem := strconv.FormatInt(elem, 10)
			overrides = overrides + " " + key + ": " + strelem + "\n"
			fmt.Printf("overrides:%s\n", overrides)
		}
		elems, ok1 := element.(string)
		if ok1 {
			overrides = overrides + " " + key + ": " + elems + "\n"
		}
	}
	fmt.Printf("Overrides:%s\n", overrides)
	return overrides
}

func executeExecCall(runner, namespace, command string) (bool, string) {
	//fmt.Println("Inside ExecuteExecCall")
	req := kubeClient.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(runner).
		Namespace(KUBEPLUS_NAMESPACE).
		SubResource("exec")

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		responseString := "Error: " + err.Error()
		fmt.Printf("Error found trying to Exec command on pod: %s \n", responseString)
		return false, responseString
	}

	parameterCodec := runtime.NewParameterCodec(scheme)
	req.VersionedParams(&corev1.PodExecOptions{
		Command:   strings.Fields(command),
		Container: CMD_RUNNER_CONTAINER,
		//Stdin:     stdin != nil,
		Stdout: true,
		Stderr: true,
		TTY:    false,
	}, parameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(cfg, "POST", req.URL())
	if err != nil {
		responseString := "Error: " + err.Error()
		fmt.Printf("Error found trying to Exec command on pod: %s \n", responseString)
		return false, responseString
	}

	var (
		execOut bytes.Buffer
		execErr bytes.Buffer
	)

	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: &execOut,
		Stderr: &execErr,
		Tty:    false,
	})
	if err != nil {
		fmt.Printf("StdOutput: %s\n", execOut.String())
		responseString := "Error: " + execErr.String()
		fmt.Printf("StdErr: %s\n", responseString)
		fmt.Printf("The command %s returned False : %s \n", command, err.Error())
		return false, responseString
	}

	responseString := execOut.String()
	//fmt.Printf("Output! :%v\n", responseString)
	return true, responseString
}
