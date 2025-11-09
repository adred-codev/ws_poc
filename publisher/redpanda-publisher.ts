import { Kafka, Producer, Message } from 'kafkajs';
import {
  EVENT_TYPE_TOPICS,
  TokenEvent,
  EventType,
} from './types/event-types.js';

export class RedpandaPublisher {
  private kafka: Kafka;
  private producer: Producer;
  private isConnected: boolean = false;
  private publishCount: number = 0;
  private lastLogTime: number = 0;
  private lastPublishCount: number = 0;
  private statsIntervalId?: NodeJS.Timeout;

  constructor(brokers: string[]) {
    this.kafka = new Kafka({
      clientId: 'odin-publisher',
      brokers,
      retry: {
        initialRetryTime: 100,
        retries: 8,
      },
    });

    this.producer = this.kafka.producer({
      allowAutoTopicCreation: false,
      maxInFlightRequests: 5,
    });
  }

  async connect(): Promise<void> {
    if (this.isConnected) {
      return;
    }

    await this.producer.connect();
    this.isConnected = true;
    this.publishCount = 0;
    this.lastLogTime = Date.now();
    this.lastPublishCount = 0;

    console.log('[RedpandaPublisher] Connected to Redpanda');

    // Start periodic stats logging (every 10 seconds)
    this.statsIntervalId = setInterval(() => {
      const now = Date.now();
      const elapsedSeconds = (now - this.lastLogTime) / 1000;
      const eventsPublished = this.publishCount - this.lastPublishCount;
      const actualRate = eventsPublished / elapsedSeconds;

      if (eventsPublished > 0) {
        console.log(
          `[RedpandaPublisher] Published ${eventsPublished} events in last ${elapsedSeconds.toFixed(1)}s (${actualRate.toFixed(1)} events/sec) | Total: ${this.publishCount}`
        );
      }

      this.lastLogTime = now;
      this.lastPublishCount = this.publishCount;
    }, 10000); // Log every 10 seconds
  }

  async disconnect(): Promise<void> {
    if (!this.isConnected) {
      return;
    }

    if (this.statsIntervalId) {
      clearInterval(this.statsIntervalId);
      this.statsIntervalId = undefined;
    }

    await this.producer.disconnect();
    this.isConnected = false;
    console.log(
      `[RedpandaPublisher] Disconnected from Redpanda after publishing ${this.publishCount} total events`
    );
  }

  /**
   * Publish a single token event
   */
  async publishEvent(event: TokenEvent): Promise<void> {
    if (!this.isConnected) {
      throw new Error('Publisher not connected. Call connect() first.');
    }

    const topic = EVENT_TYPE_TOPICS[event.type];
    if (!topic) {
      throw new Error(`Unknown event type: ${event.type}`);
    }

    const message: Message = {
      key: event.tokenId, // Partition by token ID
      value: JSON.stringify({
        type: event.type,
        timestamp: event.timestamp,
        data: event.data,
      }),
      timestamp: event.timestamp.toString(),
    };

    await this.producer.send({
      topic,
      messages: [message],
    });

    this.publishCount++;

    // Log every publish just like NATS did
    console.log(
      `[RedpandaPublisher] Published ${event.type} for token ${event.tokenId} to ${topic}`
    );
  }

  /**
   * Publish multiple events in a batch (more efficient)
   */
  async publishBatch(events: TokenEvent[]): Promise<void> {
    if (!this.isConnected) {
      throw new Error('Publisher not connected. Call connect() first.');
    }

    if (events.length === 0) {
      return;
    }

    // Group events by topic
    const eventsByTopic = new Map<string, Message[]>();

    for (const event of events) {
      const topic = EVENT_TYPE_TOPICS[event.type];
      if (!topic) {
        console.warn(
          `[RedpandaPublisher] Unknown event type: ${event.type}, skipping`
        );
        continue;
      }

      if (!eventsByTopic.has(topic)) {
        eventsByTopic.set(topic, []);
      }

      const message: Message = {
        key: event.tokenId,
        value: JSON.stringify({
          type: event.type,
          timestamp: event.timestamp,
          data: event.data,
        }),
        timestamp: event.timestamp.toString(),
      };

      eventsByTopic.get(topic)!.push(message);
    }

    // Send batches to each topic
    const sendPromises = Array.from(eventsByTopic.entries()).map(
      ([topic, messages]) =>
        this.producer.send({
          topic,
          messages,
        })
    );

    await Promise.all(sendPromises);
  }

  /**
   * Health check
   */
  isHealthy(): boolean {
    return this.isConnected;
  }
}
