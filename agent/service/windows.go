//go:build windows

// service/windows.go - Windows 서비스 래퍼
package service

import (
	"log"

	"golang.org/x/sys/windows/svc"
)

const ServiceName = "OWLmon-Agent"

// handler는 Windows 서비스 이벤트를 처리합니다.
type handler struct {
	run func() func() // run()은 정지 함수를 반환합니다
}

func (h *handler) Execute(args []string, r <-chan svc.ChangeRequest, s chan<- svc.Status) (bool, uint32) {
	s <- svc.Status{State: svc.StartPending}

	stop := h.run()

	s <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	for c := range r {
		switch c.Cmd {
		case svc.Stop, svc.Shutdown:
			s <- svc.Status{State: svc.StopPending}
			stop()
			return false, 0
		}
	}
	return false, 0
}

// Run은 Windows 서비스로 실행합니다. run()은 에이전트 시작 함수, 반환값은 정지 함수입니다.
func Run(run func() func()) error {
	log.Println("Windows 서비스 모드로 실행 중...")
	return svc.Run(ServiceName, &handler{run: run})
}

// IsService는 현재 프로세스가 Windows 서비스로 실행 중인지 반환합니다.
func IsService() bool {
	ok, _ := svc.IsWindowsService()
	return ok
}
