package notifier

// NullDownloadLogger is a null implementation of the DownloadLogger interface.
type NullDownloadLogger struct{}

// NotifyStatus implements the Notifier interface.
func (p NullDownloadLogger) NotifyStatus(_ string, _ DownloadStats) {
}

// NotifyStart implements the Notifier interface.
func (p NullDownloadLogger) NotifyStart(_ string) {
}

// NotifyFinish implements the Notifier interface.
func (p NullDownloadLogger) NotifyFinish(_ string) {
}
