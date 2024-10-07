import axios from "axios";

export const API = axios.create({
  baseURL: `${process.env.INTERNAL_BACKEND_BASE_URL}/api`,
});
