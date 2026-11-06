//go:build !windows

// service/unix.go - macOS / Linux 스텁 (Windows 서비스 기능 미지원)
package service

const ServiceName = "OWLmon-Server"

// Run은 Unix에서 Windows 서비스를 지원하지 않으므로 즉시 run()을 실행합니다.
func Run(run func() func()) error {
	// IsService가 항상 false라 실제로는 호출되지 않음 (종료 처리는 main의 시그널 대기가 담당)
	run()
	select {}
}

// IsService는 Unix에서 항상 false를 반환합니다.
func IsService() bool {
	return false
}
