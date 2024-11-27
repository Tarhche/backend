type Response = {
  data?: any;
};

export class APIClientError extends Error {
  public statusCode: number;
  public response?: Response = {};

  constructor(message: string, statusCode: number, response?: Response) {
    super(message);
    this.name = "APIClientError";
    this.statusCode = statusCode;
    this.response = response;
  }
}
