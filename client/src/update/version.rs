//! 版本号解析与比较（STAGE-99）。按语义化版本比较；预发布版本不作为升级目标。

use semver::Version;

/// 当前客户端的版本号，来自 `Cargo.toml`。
pub fn current() -> Version {
    Version::parse(env!("CARGO_PKG_VERSION")).expect("Cargo.toml 的版本号应是合法的语义化版本")
}

/// 解析发布标签，如 `v0.2.0` → `0.2.0`；不是合法版本号时返回 `None`。
pub fn parse_tag(tag: &str) -> Option<Version> {
    Version::parse(tag.trim().trim_start_matches('v')).ok()
}

/// `latest` 是否应提示升级：比当前版本新，且本身不是预发布版本。
pub fn is_upgrade(current: &Version, latest: &Version) -> bool {
    latest.pre.is_empty() && latest > current
}

#[cfg(test)]
mod tests {
    use super::*;

    fn v(s: &str) -> Version {
        Version::parse(s).unwrap()
    }

    #[test]
    fn parse_tag_accepts_v_prefix() {
        assert_eq!(parse_tag("v0.2.0"), Some(v("0.2.0")));
        assert_eq!(parse_tag(" 1.10.3 "), Some(v("1.10.3")));
        assert_eq!(parse_tag("latest"), None);
        assert_eq!(parse_tag(""), None);
    }

    #[test]
    fn newer_stable_version_is_upgrade() {
        assert!(is_upgrade(&v("0.1.0"), &v("0.2.0")));
        assert!(is_upgrade(&v("0.9.0"), &v("0.10.0")), "按数字比较，不按字符串");
        assert!(!is_upgrade(&v("0.2.0"), &v("0.2.0")));
        assert!(!is_upgrade(&v("0.3.0"), &v("0.2.0")));
    }

    #[test]
    fn prerelease_is_never_offered() {
        assert!(!is_upgrade(&v("0.1.0"), &v("0.2.0-rc.1")));
        assert!(!is_upgrade(&v("0.1.0"), &v("0.2.0-beta")));
    }

    #[test]
    fn current_is_valid() {
        assert_eq!(current().to_string(), env!("CARGO_PKG_VERSION"));
    }
}
