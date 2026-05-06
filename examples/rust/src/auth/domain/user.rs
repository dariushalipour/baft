use serde::{Deserialize, Serialize};
use std::time::SystemTime;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct BaseEntity {
    pub id: String,
    pub created_at: SystemTime,
    pub updated_at: SystemTime,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct User {
    #[serde(flatten)]
    pub base: BaseEntity,
    pub email: String,
    pub name: String,
    pub role: UserRole,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum UserRole {
    Admin,
    Member,
    Viewer,
}

pub trait UserRepository {
    fn find_by_id(&self, id: &str) -> Result<Option<User>, DomainError>;
    fn find_by_email(&self, email: &str) -> Result<Option<User>, DomainError>;
    fn create(&self, user: &User) -> Result<(), DomainError>;
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Token {
    pub value: String,
    pub user_id: String,
    pub role: String,
    pub expires_at: SystemTime,
}

pub trait TokenRepository {
    fn save(&self, token: &Token) -> Result<(), DomainError>;
    fn find_by_value(&self, value: &str) -> Result<Option<Token>, DomainError>;
    fn revoke(&self, value: &str) -> Result<(), DomainError>;
}
