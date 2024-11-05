"use client";
import {forwardRef} from "react";
import {useFormStatus} from "react-dom";
import {Button, ButtonProps, PolymorphicComponentProps} from "@mantine/core";

export const FormButton = forwardRef<
  HTMLButtonElement,
  PolymorphicComponentProps<"button", ButtonProps>
>(function (props, ref) {
  const {children, ...rest} = props;
  const {pending} = useFormStatus();
  return (
    <Button {...rest} loading={pending} ref={ref} type="submit">
      {children}
    </Button>
  );
});

FormButton.displayName = "FormButton";
