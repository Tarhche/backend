<template>
    <div>
        <Navbar></Navbar>
        <slot />
        <Footer></Footer>
    </div>
</template>

<script setup>
import "~/assets/scss/public/main.scss"

useHead({
  titleTemplate:(title) => title ? ` طرح چه | ${ title } ` : "طرح چه"
})
</script>
