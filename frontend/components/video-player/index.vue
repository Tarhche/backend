<template>
  <video id="video-player" class="video-js" controls preload="auto" :poster="poster" data-setup="{}">
    <source :src="video" type="video/mp4"/>
    <p class="vjs-no-js">
      To view this video please enable JavaScript, and consider upgrading to a web browser that
      <a href="https://videojs.com/html5-video-support/" target="_blank">supports HTML5 video</a>
    </p>
  </video>
</template>

<script setup lang="ts">
// import videojs from "video.js";

const {video, poster} = defineProps({
  video: String,
  poster: String,
})


// onMounted(() => {
//   const player = videojs("video-player", {}, () => {
//     player.log('onPlayerReady');
//   })
// })
</script>
