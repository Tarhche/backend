<template>
  <vue-plyr :options="options" >
    <video id="video-player" class="video-js" controls preload="auto" :poster="poster">
      <source :src="video" type="video/mp4"/>
      <p class="vjs-no-js">
        To view this video please enable JavaScript, and consider upgrading to a web browser that
        <a href="https://videojs.com/html5-video-support/" target="_blank">supports HTML5 video</a>
      </p>
    </video>
  </vue-plyr>
</template>

<script setup lang="ts">
const {video, poster} = defineProps({
  video: String,
  poster: String,
})

const options = {quality: {default: '1080p'}}
</script>
