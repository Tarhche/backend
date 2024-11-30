type Response = {
  data?: any;
};

export class DALDriverError extends Error {
  public statusCode: number;
  public response?: Response = {};

  constructor(message: string, statusCode: number, response?: Response) {
    super(message);
    this.name = "DALClientError";
    this.statusCode = statusCode;
    this.response = response;
  }
}
