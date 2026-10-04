use axum::{
    body::Body,
    http::{Request, Response, Uri},
};
use hyper_util::client::legacy::connect::HttpConnector;
use hyper_util::rt::TokioExecutor;
use http_body_util::BodyExt;
use std::collections::HashMap;

use crate::config::RouteConfig;
use crate::error::AppError;
use crate::router::is_hop_by_hop;

pub async fn forward_request(
    mut parts: axum::http::request::Parts,
    body: Body,
    route: &RouteConfig,
    original_path: &str,
) -> Result<Response<Body>, AppError> {
    let path = if route.strip_prefix.unwrap_or(false) {
        if let Some(ref prefix) = route.match_.path_prefix {
            original_path
                .strip_prefix(prefix)
                .unwrap_or(original_path)
                .to_string()
        } else {
            original_path.to_string()
        }
    } else {
        original_path.to_string()
    };

    let upstream_uri = format!("{}{}", route.upstream, path);
    let uri: Uri = upstream_uri.parse().map_err(|_| {
        AppError::ProxyError("Invalid upstream URI".to_string())
    })?;

    parts.uri = uri;

    filter_headers(&mut parts.headers, &route.remove_request_headers);
    add_headers(&mut parts.headers, &route.add_request_headers);

    let new_req = Request::from_parts(parts, body);

    let connector = HttpConnector::new();
    let client = hyper_util::client::legacy::Client::builder(TokioExecutor::new())
        .build(connector);

    let response = client
        .request(new_req)
        .await
        .map_err(|e| AppError::UpstreamError(format!("Failed to forward request: {}", e)))?;

    let (mut resp_parts, resp_body) = response.into_parts();

    filter_headers(&mut resp_parts.headers, &route.remove_response_headers);
    add_headers(&mut resp_parts.headers, &route.add_response_headers);

    let body_bytes = resp_body.collect().await
        .map_err(|e| AppError::UpstreamError(format!("Failed to collect response body: {}", e)))?
        .to_bytes();

    Ok(Response::from_parts(resp_parts, Body::from(body_bytes)))
}

fn filter_headers(
    headers: &mut axum::http::HeaderMap,
    remove_list: &Option<Vec<String>>,
) {
    if let Some(to_remove) = remove_list {
        for header_name in to_remove {
            headers.remove(header_name);
        }
    }

    let to_remove: Vec<String> = headers
        .keys()
        .filter(|name| is_hop_by_hop(name.as_str()))
        .map(|name| name.to_string())
        .collect();

    for header_name in to_remove {
        headers.remove(&header_name);
    }
}

fn add_headers(
    headers: &mut axum::http::HeaderMap,
    add_map: &Option<HashMap<String, String>>,
) {
    if let Some(to_add) = add_map {
        for (key, value) in to_add {
            if let Ok(header_value) = value.parse() {
                headers.insert(
                    axum::http::HeaderName::try_from(key.as_str())
                        .unwrap_or_else(|_| axum::http::header::HOST),
                    header_value,
                );
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_filter_hop_by_hop_headers() {
        let mut headers = axum::http::HeaderMap::new();
        headers.insert(
            axum::http::header::CONNECTION,
            "keep-alive".parse().unwrap(),
        );
        headers.insert(
            axum::http::header::CONTENT_TYPE,
            "application/json".parse().unwrap(),
        );

        filter_headers(&mut headers, &None);

        assert!(headers.get(axum::http::header::CONNECTION).is_none());
        assert!(headers.get(axum::http::header::CONTENT_TYPE).is_some());
    }

    #[test]
    fn test_remove_specific_headers() {
        let mut headers = axum::http::HeaderMap::new();
        headers.insert(
            axum::http::header::AUTHORIZATION,
            "Bearer token".parse().unwrap(),
        );
        headers.insert(
            axum::http::header::CONTENT_TYPE,
            "application/json".parse().unwrap(),
        );

        let remove_list = Some(vec!["authorization".to_string()]);
        filter_headers(&mut headers, &remove_list);

        assert!(headers.get(axum::http::header::AUTHORIZATION).is_none());
        assert!(headers.get(axum::http::header::CONTENT_TYPE).is_some());
    }

    #[test]
    fn test_add_headers() {
        let mut headers = axum::http::HeaderMap::new();
        let add_map = Some(
            vec![
                ("x-forwarded-for".to_string(), "192.168.1.1".to_string()),
                ("x-custom-header".to_string(), "custom-value".to_string()),
            ]
            .into_iter()
            .collect(),
        );

        add_headers(&mut headers, &add_map);

        assert!(headers.get("x-forwarded-for").is_some());
        assert!(headers.get("x-custom-header").is_some());
    }
}
