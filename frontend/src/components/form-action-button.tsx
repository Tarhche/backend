"use client";
import {ReactNode} from "react";
import {forwardRef} from "react";
import {useFormStatus} from "react-dom";
import {
  ActionIcon,
  ActionIconProps,
  PolymorphicComponentProps,
} from "@mantine/core";

type Props = PolymorphicComponentProps<"button", ActionIconProps> & {
  loadingPlaceholder: ReactNode;
};

export const FormActionButton = forwardRef<HTMLButtonElement, Props>(
  function (props, ref) {
    const {children, loadingPlaceholder, ...rest} = props;
    const {pending} = useFormStatus();
    return (
      <ActionIcon
        {...rest}
        ref={ref}
        type={pending ? "button" : "submit"}
        style={{
          cursor: pending ? "progress" : "pointer",
        }}
      >
        {pending ? loadingPlaceholder : children}
      </ActionIcon>
    );
  },
);

FormActionButton.displayName = "FormActionButton";
