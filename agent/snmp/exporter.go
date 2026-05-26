package snmp

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// RegisterMetrics는 Poller가 가진 최신 결과를 OTLP 메트릭으로 노출합니다.
// 폴링 결과 (Poller.Results())를 매 콜백마다 읽어서 Observable로 보고.
//
// 노출되는 메트릭:
//   snmp.device.up           — 0/1 (장비 응답 여부)
//   snmp.device.uptime       — seconds
//   snmp.device.response_ms  — 마지막 폴링 소요 시간
//   snmp.printer.supply.percent  — 토너/잉크 잔량 %
//   snmp.printer.total_pages     — 누적 인쇄 페이지
//   snmp.switch.if.oper_up       — 0/1 (포트 up/down)
//   snmp.switch.if.in_bytes      — 누적 수신
//   snmp.switch.if.out_bytes     — 누적 송신
func RegisterMetrics(meter metric.Meter, poller *Poller) error {
	up, err := meter.Float64ObservableGauge(
		"snmp.device.up",
		metric.WithDescription("SNMP 장비 응답 여부 (1=OK, 0=실패)"),
	)
	if err != nil {
		return fmt.Errorf("snmp.device.up 생성 실패: %w", err)
	}
	uptime, err := meter.Float64ObservableGauge(
		"snmp.device.uptime",
		metric.WithDescription("장비 가동 시간 (초)"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return err
	}
	respMs, err := meter.Int64ObservableGauge(
		"snmp.device.response_ms",
		metric.WithDescription("마지막 SNMP 폴링 소요 시간 (ms)"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return err
	}
	supplyPct, err := meter.Float64ObservableGauge(
		"snmp.printer.supply.percent",
		metric.WithDescription("프린터 토너/잉크 잔량 (%)"),
		metric.WithUnit("%"),
	)
	if err != nil {
		return err
	}
	totalPages, err := meter.Int64ObservableGauge(
		"snmp.printer.total_pages",
		metric.WithDescription("프린터 누적 인쇄 페이지"),
	)
	if err != nil {
		return err
	}
	ifUp, err := meter.Float64ObservableGauge(
		"snmp.switch.if.oper_up",
		metric.WithDescription("스위치 포트 up/down (1=up)"),
	)
	if err != nil {
		return err
	}
	ifIn, err := meter.Int64ObservableCounter(
		"snmp.switch.if.in_bytes",
		metric.WithDescription("스위치 포트 누적 수신 바이트"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return err
	}
	ifOut, err := meter.Int64ObservableCounter(
		"snmp.switch.if.out_bytes",
		metric.WithDescription("스위치 포트 누적 송신 바이트"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return err
	}

	_, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		for _, r := range poller.Results() {
			devAttr := attribute.String("device", r.Device.Name)
			ipAttr := attribute.String("device_ip", r.Device.IP)
			typeAttr := attribute.String("device_type", r.Device.Type)
			descAttr := attribute.String("device_descr", r.SysDescr)

			upVal := 0.0
			if r.Up {
				upVal = 1.0
			}
			baseAttrs := metric.WithAttributes(devAttr, ipAttr, typeAttr, descAttr)
			o.ObserveFloat64(up, upVal, baseAttrs)
			o.ObserveFloat64(uptime, r.UptimeSec, baseAttrs)
			o.ObserveInt64(respMs, r.ResponseMs, baseAttrs)

			// 프린터 메트릭
			for _, s := range r.Supplies {
				if s.MaxCapacity <= 0 {
					continue // unknown/infinite — % 계산 불가
				}
				attrs := metric.WithAttributes(
					devAttr, ipAttr, typeAttr,
					attribute.String("supply", s.Description),
					attribute.Int("supply_index", s.Index),
				)
				o.ObserveFloat64(supplyPct, s.PercentLeft, attrs)
			}
			if r.TotalPages > 0 {
				o.ObserveInt64(totalPages, r.TotalPages, baseAttrs)
			}

			// 스위치 메트릭
			for _, ifs := range r.Interfaces {
				attrs := metric.WithAttributes(
					devAttr, ipAttr, typeAttr,
					attribute.String("if_name", ifs.Name),
					attribute.Int("if_index", ifs.Index),
				)
				upV := 0.0
				if ifs.OperUp {
					upV = 1.0
				}
				o.ObserveFloat64(ifUp, upV, attrs)
				o.ObserveInt64(ifIn, int64(ifs.InBytes), attrs)
				o.ObserveInt64(ifOut, int64(ifs.OutBytes), attrs)
			}
		}
		return nil
	}, up, uptime, respMs, supplyPct, totalPages, ifUp, ifIn, ifOut)

	return err
}
