import {AxiosResponse} from "axios";

export class APIClientError extends Error {
  public statusCode: number;
  public metadata?: Record<string, any> = {};

  constructor(message: string, statusCode: number, metadata?: AxiosResponse) {
    super(message);
    this.name = "APIClientError";
    this.statusCode = statusCode;
    this.metadata = metadata;
  }
}

// 4xx status codes
export class APIClientBadRequestError extends APIClientError {
  constructor(message: string, metadata?: AxiosResponse) {
    super(message, 400, metadata);
  }
}

export class APIClientForbiddenError extends APIClientError {
  constructor(message: string, metadata?: AxiosResponse) {
    super(message, 403, metadata);
  }
}

export class APIClientUnauthorizedError extends APIClientError {
  constructor(message: string, metadata?: AxiosResponse) {
    super(message, 401, metadata);
  }
}

export class APIClienNotFoundError extends APIClientError {
  constructor(message: string, metadata?: AxiosResponse) {
    super(message, 404, metadata);
  }
}

export class APIClientUnprocessableEntity extends APIClientError {
  constructor(message: string, metadata?: any) {
    super(message, 422, metadata);
  }
}

// 5xx status codes
export class APIClientInternalServerError extends APIClientError {
  constructor(message: string, metadata?: AxiosResponse) {
    super(message, 500, metadata);
  }
}
