import express, { Request, Response } from 'express';
import cors from 'cors';
import { RedpandaPublisher } from './redpanda-publisher.js';
import { EventSimulator } from './event-simulator.js';

export class ApiServer {
  private app: express.Application;
  private publisher: RedpandaPublisher;
  private simulator: EventSimulator | null = null;
  private currentRate: number = 0;

  constructor(publisher: RedpandaPublisher, port: number = 3001) {
    this.app = express();
    this.publisher = publisher;

    this.app.use(cors());
    this.app.use(express.json());

    this.setupRoutes();
    this.app.listen(port, () => {
      console.log(`[ApiServer] Listening on port ${port}`);
    });
  }

  private setupRoutes(): void {
    // Health check
    this.app.get('/health', (req: Request, res: Response) => {
      res.json({
        status: 'ok',
        publisher: this.publisher.isHealthy() ? 'connected' : 'disconnected',
        simulator: this.simulator ? 'running' : 'stopped',
        currentRate: this.currentRate,
      });
    });

    // Start event generation
    this.app.post('/start', async (req: Request, res: Response) => {
      try {
        const { rate = 100, tokenIds = [] } = req.body;

        if (!Array.isArray(tokenIds) || tokenIds.length === 0) {
          return res.status(400).json({
            error: 'tokenIds array is required and must not be empty',
          });
        }

        if (this.simulator) {
          this.simulator.stop();
        }

        this.simulator = new EventSimulator(tokenIds);
        this.currentRate = rate;

        this.simulator.start(rate, async event => {
          try {
            await this.publisher.publishEvent(event);
          } catch (error) {
            console.error('[ApiServer] Failed to publish event:', error);
          }
        });

        res.json({
          status: 'started',
          rate,
          tokenCount: tokenIds.length,
        });
      } catch (error) {
        res.status(500).json({
          error: error instanceof Error ? error.message : 'Unknown error',
        });
      }
    });

    // Stop event generation
    this.app.post('/stop', (req: Request, res: Response) => {
      try {
        if (this.simulator) {
          this.simulator.stop();
          this.simulator = null;
          this.currentRate = 0;
        }

        res.json({
          status: 'stopped',
        });
      } catch (error) {
        res.status(500).json({
          error: error instanceof Error ? error.message : 'Unknown error',
        });
      }
    });

    // Update rate
    this.app.post('/rate', (req: Request, res: Response) => {
      try {
        const { rate } = req.body;

        if (typeof rate !== 'number' || rate <= 0) {
          return res.status(400).json({
            error: 'rate must be a positive number',
          });
        }

        if (!this.simulator) {
          return res.status(400).json({
            error: 'Simulator not running. Call /start first.',
          });
        }

        // Restart with new rate
        const tokenIds = (this.simulator as any).tokenIds; // Access private field
        this.simulator.stop();

        this.simulator = new EventSimulator(tokenIds);
        this.currentRate = rate;

        this.simulator.start(rate, async event => {
          try {
            await this.publisher.publishEvent(event);
          } catch (error) {
            console.error('[ApiServer] Failed to publish event:', error);
          }
        });

        res.json({
          status: 'updated',
          rate,
        });
      } catch (error) {
        res.status(500).json({
          error: error instanceof Error ? error.message : 'Unknown error',
        });
      }
    });

    // Get current status
    this.app.get('/status', (req: Request, res: Response) => {
      res.json({
        isRunning: this.simulator !== null,
        currentRate: this.currentRate,
        publisherHealthy: this.publisher.isHealthy(),
      });
    });
  }
}
