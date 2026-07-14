# Codex Task: MeshClaw 컨테이너 헬스 + 로그 분석 + 자가치유 확장

## 역할
너는 MeshClaw(Go 1.23, module `github.com/meshclaw/meshclaw`) 레포의 시니어 백엔드 엔지니어다.
기존 코드 스타일/패턴을 **그대로 따라** 기능을 확장한다. 새 프레임워크 도입 금지.

## 절대 규칙 (중요)
1. **코드 + 단위테스트까지만 작성**한다. 실제 18대 서버 배포/실행은 절대 하지 마라(사람이 한다).
2. 모든 외부 명령 실행은 **policy gate**를 거쳐야 한다 (`internal/policy/policy.go` 패턴 준수).
3. 모든 조치(action)는 **evidence**로 기록한다 (`internal/evidence/store.go` 패턴 준수).
4. 기존 스키마는 **하위호환 유지**. 새 필드는 `omitempty`로 추가하고 schema_version을 올린다.
5. 베어메탈 노드 대상 위험 액션은 기본 `mode: propose`(승인대기), 컨테이너 대상만 `auto_safe` 허용.

## 작업 대상 파일 (실측 확인됨)
- 스키마/수집: `internal/nodestate/nodestate.go` (DockerState, DockerContainer, DockerPort 이미 존재)
- 워크로드 분류: `internal/nodestate/workload.go` (classifyProcessPurpose 패턴 참고)
- 수집 진입점: `cmd/meshclaw/main.go` (agent collect)
- 정책: `internal/policy/policy.go`
- 증거: `internal/evidence/store.go`
- 조정 루프: `internal/reconciler/`
- 자가치유 참고: 테스트에 `autoheal-apply-safe`, `disk_cleanup`, `mode: auto_safe` 패턴 존재
- 스키마 문서: `docs/NODE_REPORT_SCHEMA.md` (v2 → v3로 갱신)

## 구현할 4개 모듈

### 모듈 1: 컨테이너 헬스 확장
- `DockerContainer` 구조체에 필드 추가: `HealthStatus`(healthy/unhealthy/starting/none), `RestartCount int`, `OOMKilled bool`, `ExitCode int`, `StartedAt`
- 수집: `docker inspect`/`docker ps --format`로 위 값 파싱 (docker 미설치 시 graceful skip)
- crash loop 탐지: RestartCount 급증 또는 unhealthy를 `health` 섹션 경고로 롤업

### 모듈 2: 로그 수집·분석
- 새 패키지 `internal/logscan/` 생성
- 소스: 호스트 `journalctl -p err --since`, 컨테이너 `docker logs --tail N --since`
- 패턴 탐지: OOM(`Out of memory`,`OOMKilled`), crash loop, 인증실패 급증, 5xx 급증
- 결과를 보고서 새 섹션 `log_findings`로 추가 (severity, source, pattern, count, sample)
- **로그 본문은 redact**(이메일/IP/토큰 마스킹) 후 sample만 보관

### 모듈 3: 컨테이너 제어 액션 + 자가치유
- 액션: `container_restart`, `container_recreate`, `container_pull_redeploy`
- 화이트리스트 기반 auto-apply: 컨테이너 unhealthy/exited면 `auto_safe`로 restart 시도
- verify: 조치 후 재수집해 healthy 확인, 실패 시 **자동 롤백 훅** 호출
- 모든 단계 evidence 기록 (command, stdout, exit, before/after diff)

### 모듈 4 (선택): VM 라이프사이클 스텁
- `internal/vmlifecycle/` 인터페이스만 정의 (libvirt/proxmox 어댑터 자리)
- 실제 구현은 TODO 주석으로 남기고 인터페이스+테스트만

## 산출물
1. 위 변경에 대한 **단위테스트** (기존 `*_test.go` 스타일, table-driven)
2. `docs/NODE_REPORT_SCHEMA.md` v3 갱신 (새 섹션/필드 문서화)
3. `CHANGELOG.md` 항목 추가
4. `go build ./...` 와 `go test ./...` 통과
5. **PR 단위로 모듈별 분리** (모듈1 → 2 → 3 → 4 순서, 각각 독립 빌드 가능)

## 검증 명령
```
go build ./...
go test ./internal/nodestate/... ./internal/logscan/... ./internal/policy/... ./internal/evidence/...
go vet ./...
```

## 시작점
먼저 `internal/nodestate/nodestate.go`의 DockerContainer와 `docs/NODE_REPORT_SCHEMA.md`를 읽고,
모듈1부터 최소 변경으로 시작하라. 큰 리팩터링 금지, 기존 함수 시그니처 보존.
