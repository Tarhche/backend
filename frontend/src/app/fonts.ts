import {Vazirmatn, Roboto_Mono} from "next/font/google";

export const vazir = Vazirmatn({
  display: "swap",
  weight: ["300", "500", "700", "900"],
  style: ["normal"],
  subsets: ["latin", "latin-ext"],
});

export const roboto_mono = Roboto_Mono({
  subsets: ["vietnamese"],
  style: ["italic", "normal"],
  weight: ["100", "300", "500", "700"],
  display: "swap",
});
