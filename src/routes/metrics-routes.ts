import express, { Request, Response } from 'express';
import { WebSocket } from 'ws';
import { metricsService, DetailedMetrics } from '../services/metrics-service';

const router = express.Router();

// CORS middleware for metrics endpoints
router.use((req, res, next) => {
  res.header('Access-Control-Allow-Origin', '*');
  res.header('Access-Control-Allow-Methods', 'GET, POST, OPTIONS');
  res.header('Access-Control-Allow-Headers', 'Origin, X-Requested-With, Content-Type, Accept, Authorization');

  if (req.method === 'OPTIONS') {
    res.sendStatus(200);
    return;
  }
  next();
});

// GET /api/metrics - Current metrics snapshot
router.get('/metrics', (req: Request, res: Response) => {
  try {
    const metrics = metricsService.getMetrics();
    res.json({
      success: true,
      timestamp: Date.now(),
      data: metrics
    });
  } catch (error) {
    res.status(500).json({
      success: false,
      error: 'Failed to retrieve metrics',
      message: error instanceof Error ? error.message : 'Unknown error'
    });
  }
});

// GET /api/metrics/history - Historical metrics
router.get('/metrics/history', (req: Request, res: Response) => {
  try {
    const hours = parseInt(req.query.hours as string) || 1;
    const maxHours = 24; // Limit to prevent memory issues
    const safeHours = Math.min(Math.max(hours, 0.1), maxHours);

    const history = metricsService.getHistory(safeHours);

    res.json({
      success: true,
      timestamp: Date.now(),
      data: {
        hours: safeHours,
        snapshots: history.length,
        history
      }
    });
  } catch (error) {
    res.status(500).json({
      success: false,
      error: 'Failed to retrieve metrics history',
      message: error instanceof Error ? error.message : 'Unknown error'
    });
  }
});

// GET /api/metrics/summary - Condensed metrics for quick checks
router.get('/metrics/summary', (req: Request, res: Response) => {
  try {
    const metrics = metricsService.getMetrics();

    const summary = {
      status: metrics.system.health,
      connections: {
        active: metrics.connections.active,
        total: metrics.connections.total,
        successRate: metrics.connections.total > 0
          ? (metrics.connections.successful / metrics.connections.total * 100).toFixed(1) + '%'
          : '0%'
      },
      performance: {
        messagesPerSecond: metrics.messages.perSecond,
        averageLatency: `${metrics.latency.average.toFixed(1)}ms`,
        errorRate: `${(metrics.errors.rate * 100).toFixed(2)}%`
      },
      system: {
        uptime: formatUptime(metrics.system.uptime),
        memory: `${metrics.system.memory.percentage.toFixed(1)}%`,
        health: metrics.system.health
      },
      components: metrics.system.components,
      loadTest: metrics.loadTest.isRunning
    };

    res.json({
      success: true,
      timestamp: Date.now(),
      data: summary
    });
  } catch (error) {
    res.status(500).json({
      success: false,
      error: 'Failed to retrieve metrics summary',
      message: error instanceof Error ? error.message : 'Unknown error'
    });
  }
});

// GET /api/health-check - Comprehensive health check
router.get('/health-check', (req: Request, res: Response) => {
  try {
    const metrics = metricsService.getMetrics();
    const checks = {
      websocket: {
        status: metrics.system.components.websocket,
        connections: metrics.connections.active,
        healthy: metrics.system.components.websocket === 'healthy'
      },
      nats: {
        status: metrics.system.components.nats,
        healthy: metrics.system.components.nats === 'connected'
      },
      publisher: {
        status: metrics.system.components.publisher,
        healthy: metrics.system.components.publisher === 'active'
      },
      system: {
        memory: metrics.system.memory.percentage,
        memoryHealthy: metrics.system.memory.percentage < 90,
        errorRate: metrics.errors.rate,
        errorRateHealthy: metrics.errors.rate < 0.05
      }
    };

    const overallHealthy = Object.values(checks).every(check => {
      if ('healthy' in check && typeof check.healthy === 'boolean') {
        return check.healthy;
      }
      if ('memoryHealthy' in check && 'errorRateHealthy' in check) {
        return check.memoryHealthy && check.errorRateHealthy;
      }
      return true;
    });

    const statusCode = overallHealthy ? 200 : 503;

    res.status(statusCode).json({
      success: overallHealthy,
      timestamp: Date.now(),
      overall: overallHealthy ? 'healthy' : 'unhealthy',
      checks,
      uptime: formatUptime(metrics.system.uptime)
    });
  } catch (error) {
    res.status(503).json({
      success: false,
      overall: 'error',
      error: 'Health check failed',
      message: error instanceof Error ? error.message : 'Unknown error'
    });
  }
});

// POST /api/metrics/test-connection - Test WebSocket connectivity
router.post('/metrics/test-connection', (req: Request, res: Response) => {
  try {
    const { url, timeout = 5000 } = req.body;
    const testUrl = url || `ws://localhost:${process.env.WS_PORT || 8080}/ws`;

    // Create a test WebSocket connection
    const testWs = new WebSocket(testUrl);
    const testStartTime = Date.now();
    let responseData: any = {};

    const timeoutHandle = setTimeout(() => {
      testWs.close();
      res.status(408).json({
        success: false,
        error: 'Connection test timeout',
        timeout: timeout
      });
    }, timeout);

    testWs.on('open', () => {
      const connectTime = Date.now() - testStartTime;
      responseData.connectionTime = connectTime;

      // Send a ping message
      testWs.send(JSON.stringify({
        type: 'ping',
        timestamp: Date.now()
      }));
    });

    testWs.on('message', (data) => {
      try {
        const message = JSON.parse(data.toString());
        if (message.type === 'pong' || message.type === 'connection:established') {
          const totalTime = Date.now() - testStartTime;

          clearTimeout(timeoutHandle);
          testWs.close();

          res.json({
            success: true,
            timestamp: Date.now(),
            test: {
              url: testUrl,
              connectionTime: responseData.connectionTime,
              totalTime,
              messageReceived: message.type,
              status: 'healthy'
            }
          });
        }
      } catch (parseError) {
        // Ignore parse errors
      }
    });

    testWs.on('error', (error) => {
      clearTimeout(timeoutHandle);
      res.status(500).json({
        success: false,
        error: 'WebSocket connection failed',
        message: error.message,
        url: testUrl
      });
    });

  } catch (error) {
    res.status(500).json({
      success: false,
      error: 'Test connection setup failed',
      message: error instanceof Error ? error.message : 'Unknown error'
    });
  }
});

// GET /api/metrics/errors - Recent errors
router.get('/metrics/errors', (req: Request, res: Response) => {
  try {
    const limit = parseInt(req.query.limit as string) || 50;
    const safeLimit = Math.min(Math.max(limit, 1), 500);

    const metrics = metricsService.getMetrics();
    const recentErrors = metrics.errors.recent
      .slice(-safeLimit)
      .reverse(); // Most recent first

    res.json({
      success: true,
      timestamp: Date.now(),
      data: {
        total: metrics.errors.total,
        rate: metrics.errors.rate,
        types: metrics.errors.types,
        recent: recentErrors
      }
    });
  } catch (error) {
    res.status(500).json({
      success: false,
      error: 'Failed to retrieve error information',
      message: error instanceof Error ? error.message : 'Unknown error'
    });
  }
});

// Utility functions
function formatUptime(milliseconds: number): string {
  const seconds = Math.floor(milliseconds / 1000);
  const minutes = Math.floor(seconds / 60);
  const hours = Math.floor(minutes / 60);
  const days = Math.floor(hours / 24);

  if (days > 0) {
    return `${days}d ${hours % 24}h ${minutes % 60}m`;
  } else if (hours > 0) {
    return `${hours}h ${minutes % 60}m ${seconds % 60}s`;
  } else if (minutes > 0) {
    return `${minutes}m ${seconds % 60}s`;
  } else {
    return `${seconds}s`;
  }
}

export default router;