use thiserror::Error;

#[derive(Debug, Error)]
#[error("auth[{code}]: {message}")]
pub struct AuthError {
    pub code: String,
    pub message: String,
}

impl AuthError {
    pub fn unauthorized() -> Self {
        Self {
            code: "unauthorized".to_string(),
            message: "authentication required".to_string(),
        }
    }

    pub fn not_found(resource: &str) -> Self {
        Self {
            code: "not_found".to_string(),
            message: format!("{} not found", resource),
        }
    }
}
