"use server";
import {redirect} from "next/navigation";
import {cookies} from "next/headers";
import {getRootUrl} from "@/lib/http";

export async function logout() {
  const cookiesStore = cookies();
  const cookiesToRemove = cookiesStore.getAll();
  cookiesToRemove.forEach(({name}) => {
    cookiesStore.set(name, "", {maxAge: -1});
  });
  redirect(getRootUrl());
}
