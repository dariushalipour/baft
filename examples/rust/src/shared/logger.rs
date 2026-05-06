use chrono::{Duration, Utc};
use serde::{Serialize, Deserialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LogEntry {
    pub level: LogLevel,
    pub message: String,
    pub context: std::collections::HashMap<String, String>,
    pub timestamp: chrono::DateTime<Utc>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum LogLevel {
    Debug,
    Info,
    Warn,
    Error,
}

pub trait Logger {
    fn debug(&self, message: &str, context: &[(&str, &str)]);
    fn info(&self, message: &str, context: &[(&str, &str)]);
    fn warn(&self, message: &str, context: &[(&str, &str)]);
    fn error(&self, message: &str, context: &[(&str, &str)]);
}

pub struct ConsoleLogger;

impl Logger for ConsoleLogger {
    fn debug(&self, message: &str, _context: &[(&str, &str)]) {
        println!("{{\"level\":\"debug\",\"message\":\"{}\"}}", message);
    }
    fn info(&self, message: &str, _context: &[(&str, &str)]) {
        println!("{{\"level\":\"info\",\"message\":\"{}\"}}", message);
    }
    fn warn(&self, message: &str, _context: &[(&str, &str)]) {
        eprintln!("{{\"level\":\"warn\",\"message\":\"{}\"}}", message);
    }
    fn error(&self, message: &str, _context: &[(&str, &str)]) {
        eprintln!("{{\"level\":\"error\",\"message\":\"{}\"}}", message);
    }
}
