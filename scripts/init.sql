CREATE DATABASE IF NOT EXISTS short_url;
USE short_url;

CREATE TABLE IF NOT EXISTS short_urls (
                                          id BIGINT PRIMARY KEY AUTO_INCREMENT,
                                          short_code VARCHAR(16) NOT NULL UNIQUE,
    long_url TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
    );