use axum::{
    extract::{Path, State},
    http::{Request, StatusCode},
    response::Response,
    routing::{delete, get, post, any},
    Json, Router,
};
use serde_json::json;
use std::sync::Arc;

use crate::config::RouteConfig;
use crate::error::AppError;
use crate::proxy::forward_request;
use crate::router::match_route;
use crate::state::AppState;

pub fn build_router(state: Arc<AppState>) -> Router {
    Router::new()
        .route("/_proxy/routes", get(list_routes))
        .route("/_proxy/routes", post(add_route))
        .route("/_proxy/routes/:index", delete(remove_route))
        .route("/_proxy/health", get(proxy_health))
        .fallback(any(proxy_handler))
        .with_state(state)
}

async fn list_routes(
    State(state): State<Arc<AppState>>,
) -> Result<Json<serde_json::Value>, AppError> {
    let routes = state.get_routes().await;
    let health_statuses = state.health_checker.get_all_statuses().await;

    let routes_with_health: Vec<serde_json::Value> = routes
        .iter()
        .enumerate()
        .map(|(idx, route)| {
            json!({
                "index": idx,
                "match": route.match_,
                "upstream": route.upstream,
                "strip_prefix": route.strip_prefix,
                "add_request_headers": route.add_request_headers,
                "remove_request_headers": route.remove_request_headers,
                "add_response_headers": route.add_response_headers,
                "remove_response_headers": route.remove_response_headers,
                "health_check": route.health_check,
                "healthy": health_statuses.get(&route.upstream).copied().unwrap_or(true),
            })
        })
        .collect();

    Ok(Json(json!({
        "routes": routes_with_health,
        "total": routes.len(),
    })))
}

async fn add_route(
    State(state): State<Arc<AppState>>,
    Json(route): Json<RouteConfig>,
) -> Result<(StatusCode, Json<serde_json::Value>), AppError> {
    if route.upstream.is_empty() {
        return Err(AppError::InvalidRoute(
            "upstream URL cannot be empty".to_string(),
        ));
    }

    if let Some(ref health_config) = route.health_check {
        let rx = state.shutdown_tx.subscribe();
        state
            .health_checker
            .spawn_health_check(route.upstream.clone(), health_config.clone(), rx);
        state
            .health_checker
            .set_status(route.upstream.clone(), true)
            .await;
    }

    state.add_route(route).await;

    Ok((
        StatusCode::CREATED,
        Json(json!({
            "message": "Route added successfully"
        })),
    ))
}

async fn remove_route(
    State(state): State<Arc<AppState>>,
    Path(index): Path<usize>,
) -> Result<Json<serde_json::Value>, AppError> {
    if state.remove_route(index).await {
        Ok(Json(json!({
            "message": format!("Route {} removed successfully", index)
        })))
    } else {
        Err(AppError::NotFound(format!(
            "Route at index {} not found",
            index
        )))
    }
}

async fn proxy_health(
    State(state): State<Arc<AppState>>,
) -> Result<Json<serde_json::Value>, AppError> {
    let routes = state.get_routes().await;
    let health_statuses = state.health_checker.get_all_statuses().await;

    let all_healthy = routes
        .iter()
        .all(|route| health_statuses.get(&route.upstream).copied().unwrap_or(true));

    Ok(Json(json!({
        "healthy": all_healthy,
        "upstreams": health_statuses,
    })))
}

async fn proxy_handler(
    State(state): State<Arc<AppState>>,
    req: Request<axum::body::Body>,
) -> Result<Response, AppError> {
    let (parts, body) = req.into_parts();

    let path = parts.uri.path().to_string();
    let host = parts.headers.get("host").and_then(|h| h.to_str().ok()).map(|s| s.to_string());

    let routes = state.get_routes().await;

    let (_, route) = match_route(&routes, host.as_deref(), &path)
        .ok_or_else(|| AppError::NotFound("No matching route found".to_string()))?;

    let is_healthy = state.health_checker.is_healthy(&route.upstream).await;
    if !is_healthy {
        return Err(AppError::UnhealthyUpstream(
            route.upstream.clone(),
        ));
    }

    forward_request(parts, body, route, &path).await
}
