//go:build windows

// service/windows.go - Windows 서비스 래퍼
package service

import (
	"log"

	"golang.org/x/sys/windows/svc"
)

const ServiceName = "OWLmon-Server"

type handler struct {
	run func() func()
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

// Run은 Windows 서비스로 실행합니다.
func Run(run func() func()) error {
	log.Println("Windows 서비스 모드로 실행 중...")
	return svc.Run(ServiceName, &handler{run: run})
}

// IsService는 현재 프로세스가 Windows 서비스로 실행 중인지 반환합니다.
func IsService() bool {
	ok, _ := svc.IsWindowsService()
	return ok
}
