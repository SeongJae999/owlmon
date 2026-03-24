//go:build !windows

// service/unix.go - macOS / Linux 스텁 (Windows 서비스 기능 미지원)
package service

const ServiceName = "OWLmon-Server"

// Run은 Unix에서 Windows 서비스를 지원하지 않으므로 즉시 run()을 실행합니다.
func Run(run func() func()) error {
	stop := run()
	// Unix에서는 서비스 이벤트 루프 없이 종료 대기
	select {}
	stop()
	return nil
}

// IsService는 Unix에서 항상 false를 반환합니다.
func IsService() bool {
	return false
}
