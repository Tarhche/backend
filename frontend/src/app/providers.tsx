import {ReactNode} from "react";
import {createTheme, MantineProvider, DirectionProvider} from "@mantine/core";
import {vazir, roboto_mono} from "./fonts";

const theme = createTheme({
  fontFamily: vazir.style.fontFamily,
  fontFamilyMonospace: roboto_mono.style.fontFamily,
  headings: {
    fontFamily: vazir.style.fontFamily,
  },
});

type Props = {
  children: ReactNode;
};

export function Providers({children}: Props) {
  return (
    <MantineProvider theme={theme}>
      <DirectionProvider initialDirection="rtl">{children}</DirectionProvider>
    </MantineProvider>
  );
}
