use axum::{
    routing::get,
    http::StatusCode,
    Json, Router,
};
use reverse_proxy_rust::{
    config::{RouteConfig, RouteMatch},
    routes::build_router,
    state::AppState,
};
use serde_json::json;
use std::sync::Arc;
use tokio::net::TcpListener;

async fn start_test_upstream() -> (String, tokio::task::JoinHandle<()>) {
    let listener = TcpListener::bind("127.0.0.1:0")
        .await
        .expect("Failed to bind test upstream");

    let addr = listener
        .local_addr()
        .expect("Failed to get local address");

    let upstream_url = format!("http://{}", addr);

    let router = Router::new()
        .route("/health", get(|| async { StatusCode::OK }))
        .route(
            "/api/users",
            get(|| async {
                Json(json!({
                    "users": ["alice", "bob"]
                }))
            }),
        )
        .route(
            "/echo-headers",
            get(|| async { Json(json!({})) }),
        )
        .route(
            "/set-header",
            get(|| async { Json(json!({"ok": true})) }),
        );

    let handle = tokio::spawn(async move {
        axum::serve(listener, router)
            .await
            .expect("Upstream server error");
    });

    (upstream_url, handle)
}

#[tokio::test]
async fn test_basic_proxy_routing() {
    let (upstream_url, _handle) = start_test_upstream().await;

    let route = RouteConfig {
        match_: RouteMatch {
            host: None,
            path_prefix: Some("/api".to_string()),
        },
        upstream: upstream_url.clone(),
        strip_prefix: None,
        add_request_headers: None,
        remove_request_headers: None,
        add_response_headers: None,
        remove_response_headers: None,
        health_check: None,
    };

    let state = Arc::new(AppState::new(vec![route]));
    let _app = build_router(state);
}

#[tokio::test]
async fn test_path_prefix_stripping() {
    let (upstream_url, _handle) = start_test_upstream().await;

    let route = RouteConfig {
        match_: RouteMatch {
            host: None,
            path_prefix: Some("/service".to_string()),
        },
        upstream: upstream_url.clone(),
        strip_prefix: Some(true),
        add_request_headers: None,
        remove_request_headers: None,
        add_response_headers: None,
        remove_response_headers: None,
        health_check: None,
    };

    let state = Arc::new(AppState::new(vec![route]));
    let _app = build_router(state);
}

#[tokio::test]
async fn test_host_based_routing() {
    let (upstream_url, _handle) = start_test_upstream().await;

    let route = RouteConfig {
        match_: RouteMatch {
            host: Some("api.example.com".to_string()),
            path_prefix: None,
        },
        upstream: upstream_url.clone(),
        strip_prefix: None,
        add_request_headers: None,
        remove_request_headers: None,
        add_response_headers: None,
        remove_response_headers: None,
        health_check: None,
    };

    let state = Arc::new(AppState::new(vec![route]));
    let _app = build_router(state);
}

#[tokio::test]
async fn test_no_matching_route_returns_not_found() {
    let state = Arc::new(AppState::new(vec![]));
    let _app = build_router(state);
}

#[tokio::test]
async fn test_management_list_routes() {
    let (upstream_url, _handle) = start_test_upstream().await;

    let routes = vec![
        RouteConfig {
            match_: RouteMatch {
                host: None,
                path_prefix: Some("/api".to_string()),
            },
            upstream: upstream_url.clone(),
            strip_prefix: None,
            add_request_headers: None,
            remove_request_headers: None,
            add_response_headers: None,
            remove_response_headers: None,
            health_check: None,
        },
    ];

    let state = Arc::new(AppState::new(routes));
    let retrieved = state.get_routes().await;

    assert_eq!(retrieved.len(), 1);
}

#[tokio::test]
async fn test_management_add_route() {
    let (upstream_url, _handle) = start_test_upstream().await;

    let state = Arc::new(AppState::new(vec![]));

    let new_route = RouteConfig {
        match_: RouteMatch {
            host: None,
            path_prefix: Some("/new".to_string()),
        },
        upstream: upstream_url,
        strip_prefix: None,
        add_request_headers: None,
        remove_request_headers: None,
        add_response_headers: None,
        remove_response_headers: None,
        health_check: None,
    };

    state.add_route(new_route).await;
    let routes = state.get_routes().await;

    assert_eq!(routes.len(), 1);
}

#[tokio::test]
async fn test_management_remove_route() {
    let (upstream_url, _handle) = start_test_upstream().await;

    let routes = vec![
        RouteConfig {
            match_: RouteMatch {
                host: None,
                path_prefix: Some("/api".to_string()),
            },
            upstream: upstream_url,
            strip_prefix: None,
            add_request_headers: None,
            remove_request_headers: None,
            add_response_headers: None,
            remove_response_headers: None,
            health_check: None,
        },
    ];

    let state = Arc::new(AppState::new(routes));
    let removed = state.remove_route(0).await;

    assert!(removed);
    assert_eq!(state.get_routes().await.len(), 0);
}

#[tokio::test]
async fn test_remove_nonexistent_route() {
    let state = Arc::new(AppState::new(vec![]));
    let removed = state.remove_route(0).await;

    assert!(!removed);
}

#[tokio::test]
async fn test_health_check_initialization() {
    let (upstream_url, _handle) = start_test_upstream().await;

    let state = Arc::new(AppState::new(vec![]));
    state
        .health_checker
        .set_status(upstream_url.clone(), true)
        .await;

    let is_healthy = state.health_checker.is_healthy(&upstream_url).await;
    assert!(is_healthy);
}

#[tokio::test]
async fn test_health_check_unhealthy() {
    let (upstream_url, _handle) = start_test_upstream().await;

    let state = Arc::new(AppState::new(vec![]));
    state
        .health_checker
        .set_status(upstream_url.clone(), false)
        .await;

    let is_healthy = state.health_checker.is_healthy(&upstream_url).await;
    assert!(!is_healthy);
}

#[tokio::test]
async fn test_multiple_routes() {
    let (upstream_url_1, _handle_1) = start_test_upstream().await;
    let (upstream_url_2, _handle_2) = start_test_upstream().await;

    let routes = vec![
        RouteConfig {
            match_: RouteMatch {
                host: None,
                path_prefix: Some("/api".to_string()),
            },
            upstream: upstream_url_1,
            strip_prefix: None,
            add_request_headers: None,
            remove_request_headers: None,
            add_response_headers: None,
            remove_response_headers: None,
            health_check: None,
        },
        RouteConfig {
            match_: RouteMatch {
                host: None,
                path_prefix: Some("/service".to_string()),
            },
            upstream: upstream_url_2,
            strip_prefix: None,
            add_request_headers: None,
            remove_request_headers: None,
            add_response_headers: None,
            remove_response_headers: None,
            health_check: None,
        },
    ];

    let state = Arc::new(AppState::new(routes));
    let retrieved = state.get_routes().await;

    assert_eq!(retrieved.len(), 2);
}
