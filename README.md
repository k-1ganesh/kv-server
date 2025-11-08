# KV Server with Cache & Database

## Introduction

This project implements a Key-Value (KV) data store that allows clients to store and retrieve values using simple HTTP requests.

To improve performance, the system uses an in-memory cache to minimize repeated database lookups, while PostgreSQL is used to ensure persistent and reliable data storage.

The main goal of the system is to provide fast data retrieval while maintaining persistent and reliable storage.

Real-world applications often require high performance for frequently accessed data and durability to avoid data loss. This system addresses both requirements effectively.

---

## System Architecture

    +-----------+
    |   Client  |
    +-----+-----+
          |
          v
    +-----+-----+
    |  HTTP     |
    |  Server   |
    +-----+-----+
          |
          v
    +-----+-----+
    |   Cache   |
    +-----+-----+
          |
          v
    +-----+-----+
    |  Database |
    +-----+-----+

---

## Components Description

| Component               | Description                                                   |
| ----------------------- | ------------------------------------------------------------- |
| **Client**              | Sends `GET` and `SET` requests                                |
| **HTTP Server**         | Parses client requests and sends responses                    |
| **Cache Layer**         | Stores frequently accessed key-value pairs for fast retrieval |
| **PostgreSQL Database** | Stores all data persistently                                  |

---

## Execution Path

### 1. GET Request

1. Server checks the key in the cache.
2. If found, value is returned immediately (fast lookup).
3. If not found, server fetches value from the database, stores it in cache, and returns the value.

### 2. SET Request

1. Server updates the value in the database.
2. Cache is updated with the new value.
3. Server returns the updated value.

---

## Database Schema

```sql
CREATE TABLE kv_store (
    key VARCHAR(255) PRIMARY KEY,
    value TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```
