# Reverse Proxy — Architecture

## Overview

_TODO: describe what this system does and the core design decisions._

## Component Diagram

```mermaid
graph TD
    Client --> Server
    Server --> Storage
```

## Sequence Diagram

```mermaid
sequenceDiagram
    participant Client
    participant Server
    participant Storage
    Client->>Server: Request
    Server->>Storage: Query
    Storage-->>Server: Result
    Server-->>Client: Response
```

## Data Flow

_TODO: describe how data moves through the system._

## Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| _TODO_ | _TODO_ |
