package controller

type statefulSetNotOwnedByServerError struct {
	ServerName      string
	ServerNamespace string
}

func (e *statefulSetNotOwnedByServerError) Error() string {
	return "statefulset is not owned by the server " + e.ServerName + "/" + e.ServerName
}

type serviceNotOwnedByServerError struct {
	ServerName      string
	ServerNamespace string
}

func (e *serviceNotOwnedByServerError) Error() string {
	return "service is not owned by the server " + e.ServerName + "/" + e.ServerName
}
