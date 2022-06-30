#version 410

uniform mat4 ProjMtx;

in vec3 Position;
in vec3 Normal;
in vec2 UV;
in vec4 Color;

out vec2 Frag_UV;
out vec4 Frag_Color;

void main()
{
    Frag_UV = UV;
    Frag_Color = vec4(1,1,1,1);
    Frag_Color = Color;
    gl_Position = ProjMtx * vec4(Position.xy, 0, 1);
}