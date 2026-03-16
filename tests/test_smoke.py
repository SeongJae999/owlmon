# CI 파이프라인 검증용 스모크 테스트
def test_import_agent() -> None:
    """에이전트 패키지 import 가능 여부 확인"""
    import src.agent  # noqa: F401


def test_import_api() -> None:
    """API 패키지 import 가능 여부 확인"""
    import src.api  # noqa: F401
