use crate::config::{RouteConfig, RouteMatch};

pub fn match_route<'a>(
    routes: &'a [RouteConfig],
    host: Option<&str>,
    path: &str,
) -> Option<(usize, &'a RouteConfig)> {
    routes.iter().enumerate().find(|(_, route)| {
        route_matches(&route.match_, host, path)
    })
}

fn route_matches(match_: &RouteMatch, host: Option<&str>, path: &str) -> bool {
    if let Some(ref required_host) = match_.host {
        match host {
            None => return false,
            Some(h) => {
                if h != required_host {
                    return false;
                }
            }
        }
    }

    if let Some(ref path_prefix) = match_.path_prefix {
        if !path.starts_with(path_prefix) {
            return false;
        }
    }

    true
}

pub fn is_hop_by_hop(header_name: &str) -> bool {
    matches!(
        header_name.to_lowercase().as_str(),
        "connection"
            | "keep-alive"
            | "transfer-encoding"
            | "te"
            | "trailer"
            | "upgrade"
    )
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_route_matches_by_path_prefix() {
        let match_ = RouteMatch {
            host: None,
            path_prefix: Some("/api".to_string()),
        };

        assert!(route_matches(&match_, None, "/api/users"));
        assert!(route_matches(&match_, None, "/api"));
        assert!(!route_matches(&match_, None, "/other"));
    }

    #[test]
    fn test_route_matches_by_host() {
        let match_ = RouteMatch {
            host: Some("example.com".to_string()),
            path_prefix: None,
        };

        assert!(route_matches(&match_, Some("example.com"), "/any/path"));
        assert!(!route_matches(&match_, Some("other.com"), "/any/path"));
        assert!(!route_matches(&match_, None, "/any/path"));
    }

    #[test]
    fn test_route_matches_by_host_and_path() {
        let match_ = RouteMatch {
            host: Some("api.example.com".to_string()),
            path_prefix: Some("/v1".to_string()),
        };

        assert!(route_matches(&match_, Some("api.example.com"), "/v1/users"));
        assert!(!route_matches(&match_, Some("api.example.com"), "/v2/users"));
        assert!(!route_matches(&match_, Some("other.com"), "/v1/users"));
    }

    #[test]
    fn test_is_hop_by_hop() {
        assert!(is_hop_by_hop("connection"));
        assert!(is_hop_by_hop("Connection"));
        assert!(is_hop_by_hop("keep-alive"));
        assert!(is_hop_by_hop("transfer-encoding"));
        assert!(is_hop_by_hop("te"));
        assert!(is_hop_by_hop("trailer"));
        assert!(is_hop_by_hop("upgrade"));
        assert!(!is_hop_by_hop("content-type"));
        assert!(!is_hop_by_hop("authorization"));
    }
}
