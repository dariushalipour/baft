use thiserror::Error;

#[derive(Debug, Error)]
#[error("billing[{code}]: {message}")]
pub struct BillingError {
    pub code: String,
    pub message: String,
}

impl BillingError {
    pub fn not_found(resource: &str) -> Self {
        Self {
            code: "not_found".to_string(),
            message: format!("{} not found", resource),
        }
    }

    pub fn conflict(message: &str) -> Self {
        Self {
            code: "conflict".to_string(),
            message: message.to_string(),
        }
    }

    pub fn validation(field: &str, message: &str) -> Self {
        Self {
            code: "validation_error".to_string(),
            message: format!("{}: {}", field, message),
        }
    }
}
